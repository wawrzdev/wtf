package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/wawrzdev/wtf/internal/config"
	"github.com/wawrzdev/wtf/internal/content"
	"github.com/wawrzdev/wtf/internal/inventory"
)

type runtime struct {
	cfg                 config.Config
	records             []Record
	cache               inventory.Cache
	cachePath, cacheDir string
	stdin               io.Reader
	stdout, stderr      io.Writer
	version             string
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			fmt.Fprint(stdout, usage)
			return nil
		case "version", "--version":
			fmt.Fprintln(stdout, version)
			return nil
		case "init":
			return initCommand(args[1:], stdout)
		case "completion":
			return completion(args[1:], stdout)
		}
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	_, _, cacheHome, e := config.Paths()
	if e != nil {
		return e
	}
	rt := runtime{cfg: cfg, cacheDir: filepath.Join(cacheHome, "wtf"), cachePath: filepath.Join(cacheHome, "wtf", "packages-v1.json"), stdin: stdin, stdout: stdout, stderr: stderr, version: version}
	if len(args) > 0 && args[0] == "cache" {
		return rt.cacheCommand(ctx, args[1:])
	}
	anns, topics, warns := content.Load(cfg.Annotations.Paths, cfg.Topics.Dirs)
	for _, w := range warns {
		fmt.Fprintf(stderr, "wtf: warning: %v\n", w)
	}
	if len(args) > 0 && args[0] == "topic" {
		rt.records = buildRecords(anns, topics, nil, true)
		return rt.topicCommand(ctx, args[1:])
	}
	rt.cache, _, e = inventory.Load(ctx, rt.cachePath, false, inventory.DefaultProviders())
	if e != nil {
		fmt.Fprintf(stderr, "wtf: warning: package cache: %v\n", e)
	}
	rt.records = buildRecords(anns, topics, inventory.AllPackages(rt.cache), true)
	if merged, walkErr := mergeTLDR(rt.records); walkErr != nil {
		fmt.Fprintf(stderr, "wtf: warning: local tldr discovery: %v\n", walkErr)
	} else {
		rt.records = merged
	}
	rt.records = enrichAnnotatedSystemPackages(ctx, rt.records)
	if len(args) > 0 && args[0] == "list" {
		return rt.list(args[1:])
	}
	return rt.lookup(ctx, args)
}

func (r *runtime) cacheCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: wtf cache <status|refresh> [--json]")
	}
	jsonOut := contains(args, "--json")
	refresh := args[0] == "refresh"
	if !refresh && args[0] != "status" {
		return fmt.Errorf("unknown cache command %q", args[0])
	}
	if refresh && len(args) != 1 {
		return errors.New("usage: wtf cache refresh")
	}
	if !refresh {
		for _, arg := range args[1:] {
			if arg != "--json" {
				return fmt.Errorf("unknown cache status option %q", arg)
			}
		}
	}
	_, statuses, e := inventory.Load(ctx, r.cachePath, refresh, inventory.DefaultProviders())
	if e != nil {
		return e
	}
	if jsonOut {
		b, _ := marshalStable(statuses)
		fmt.Fprintln(r.stdout, string(b))
		return nil
	}
	writeCacheStatus(r.stdout, statuses)
	return nil
}
func writeCacheStatus(w io.Writer, statuses []inventory.Status) {
	fmt.Fprintln(w, "PROVIDER\tAGE_SECONDS\tFINGERPRINT_STATE\tSTALE\tERROR")
	for _, s := range statuses {
		state := "current"
		if s.Changed {
			state = "changed"
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%t\t%s\n", s.Provider, s.AgeSeconds, state, s.Stale, sanitizeTSV(s.Error))
	}
}

func (r *runtime) list(args []string) error {
	jsonOut := contains(args, "--json")
	includeUnavailable := contains(args, "-a") || contains(args, "--all")
	for _, a := range args {
		if a != "--json" && a != "-a" && a != "--all" {
			return fmt.Errorf("unknown list option %q", a)
		}
	}
	visible := make([]Record, 0, len(r.records))
	for _, x := range r.records {
		if x.Available || x.Kind == "topic" || x.HasTLDR || includeUnavailable {
			visible = append(visible, x)
		}
	}
	if jsonOut {
		b, e := json.MarshalIndent(visible, "", "  ")
		if e != nil {
			return e
		}
		fmt.Fprintln(r.stdout, string(b))
		return nil
	}
	tty := isTerminalWriter(r.stdout)
	if tty {
		fmt.Fprintln(r.stdout, "KIND       COMMAND              STATUS       SUMMARY")
	}
	for _, x := range visible {
		status := "unavailable"
		if x.Available {
			status = "available"
		}
		if tty {
			fmt.Fprintf(r.stdout, "%-10s %-20s %-12s %s\n", x.Kind, x.ID, status, x.Summary)
		} else {
			fmt.Fprintf(r.stdout, "%s\t%s\t%s\t%s\n", x.Kind, x.ID, status, sanitizeTSV(x.Summary))
		}
	}
	return nil
}

func (r *runtime) lookup(ctx context.Context, args []string) error {
	localOnly := false
	var shell *ShellResolution
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			queryParts = append(queryParts, args[i+1:]...)
			i = len(args)
		case "--local-only":
			localOnly = true
		case "--shell-kind":
			if i+3 >= len(args) {
				return errors.New("invalid private shell protocol")
			}
			shell = &ShellResolution{Kind: args[i+1], Name: args[i+2], Expansion: args[i+3]}
			i += 3
		case "--shell-target":
			if shell == nil || i+1 >= len(args) {
				return errors.New("invalid private shell target")
			}
			shell.Target = args[i+1]
			i++
		case "--shell-cycle":
			if shell == nil {
				return errors.New("invalid private shell cycle")
			}
			shell.Cycle = true
		case "--shell-complex":
			if shell == nil {
				return errors.New("invalid private shell expansion")
			}
			shell.Complex = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			queryParts = append(queryParts, args[i])
		}
	}
	if len(queryParts) == 0 {
		if shell != nil {
			return errors.New("missing query")
		}
		visible := make([]Record, 0, len(r.records))
		for _, x := range r.records {
			if x.Available || x.Kind == "topic" || x.HasTLDR {
				visible = append(visible, x)
			}
		}
		return r.picker(ctx, visible, "")
	}
	query := strings.Join(queryParts, " ")
	if strings.Contains(query, "/") {
		if st, e := os.Stat(query); e == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			x := Record{ID: filepath.Base(query), Kind: "command", Available: true, Path: query}
			return r.display(ctx, x, localOnly)
		}
	}
	matches := matchRecords(r.records, query)
	if len(matches) == 0 {
		if shell != nil {
			matches = []Record{{ID: query, Kind: shell.Kind, Available: true}}
		} else {
			if p, e := exec.LookPath(query); e == nil {
				x := Record{ID: query, Kind: "command", Available: true, Path: p}
				x.Packages = inventory.ExactSystem(ctx, query, p)
				return r.display(ctx, x, localOnly)
			}
			return fmt.Errorf("no match for %q", query)
		}
	}
	if len(matches) > 1 {
		return r.picker(ctx, matches, query)
	}
	x := matches[0]
	if shell != nil {
		parseShell(shell)
		x.Shell = shell
		if shell.Name == x.ID && (shell.Kind == "alias" || shell.Kind == "function") {
			x.Available = true
		}
		if shell.Target != "" && !shell.Complex && !shell.Cycle {
			if p, e := exec.LookPath(shell.Target); e == nil {
				x.Path = p
				x.Available = true
				x.Packages = append(x.Packages, packagesFor(r.cache, shell.Target)...)
				x.Packages = append(x.Packages, inventory.ExactSystem(ctx, shell.Target, p)...)
			}
		}
	}
	if x.Available && len(x.Packages) == 0 {
		x.Packages = inventory.ExactSystem(ctx, x.ID, x.Path)
	}
	return r.display(ctx, x, localOnly)
}

func parseShell(s *ShellResolution) {
	suppliedTarget := s.Target
	s.Target = ""
	if s.Kind != "alias" && s.Kind != "function" {
		s.Complex = true
		return
	}
	if s.Cycle {
		return
	}
	target, ok := simpleShellTarget(s.Expansion)
	if s.Kind == "function" {
		if !ok {
			s.Complex = true
		}
		return
	}
	if !ok || s.Complex || (suppliedTarget != "" && suppliedTarget != target) {
		s.Complex = true
		return
	}
	s.Target = target
	if s.Target == s.Name {
		s.Cycle = true
		s.Target = ""
	}
}
func simpleShellTarget(expansion string) (string, bool) {
	if strings.TrimSpace(expansion) == "" || strings.ContainsAny(expansion, "|;&()<>`$\n\r\\\"'") {
		return "", false
	}
	fields := strings.Fields(expansion)
	if len(fields) == 0 || !safeCommandID(fields[0]) {
		return "", false
	}
	for _, field := range fields {
		for _, r := range field {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._+-/:,=@%", r)) {
				return "", false
			}
		}
	}
	if strings.Contains(fields[0], "=") {
		return "", false
	}
	return fields[0], true
}
func packagesFor(c inventory.Cache, cmd string) []inventory.Package {
	var r []inventory.Package
	for _, p := range inventory.AllPackages(c) {
		for _, x := range p.Commands {
			if x == cmd {
				r = append(r, p)
			}
		}
	}
	return r
}
func matchRecords(rs []Record, q string) []Record {
	ql := strings.ToLower(q)
	var exact []Record
	for _, r := range rs {
		if strings.EqualFold(r.ID, q) || r.Title != "" && strings.EqualFold(r.Title, q) {
			exact = append(exact, r)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	var out []Record
	for _, r := range rs {
		if strings.Contains(strings.ToLower(r.ID), ql) || strings.Contains(strings.ToLower(r.Title), ql) || strings.Contains(strings.ToLower(r.Summary), ql) {
			out = append(out, r)
		}
	}
	return out
}
func (r *runtime) display(ctx context.Context, x Record, local bool) error {
	doc := compose(ctx, x, docOptions{LocalOnly: local, CheatEnabled: r.cfg.Cheat.Enabled, CacheDir: r.cacheDir})
	return render(ctx, r.stdout, r.stderr, doc, r.cfg.Pager)
}

func (r *runtime) picker(ctx context.Context, records []Record, query string) error {
	fzf, e := exec.LookPath("fzf")
	if e != nil {
		return errors.New("fzf is required for interactive selection; use `wtf list` or install fzf")
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	preview := shellQuote(exe) + " --local-only -- {1}"
	args := []string{"--delimiter=\t", "--with-nth=2", "--layout=reverse", "--wrap", "--prompt=wtf ▸ ", "--header=enter: docs   ·   esc: quit", "--preview", preview, "--preview-window=right,55%,wrap,border-left"}
	if query != "" {
		args = append(args, "--query", query)
	}
	cmd := exec.CommandContext(ctx, fzf, args...)
	var in strings.Builder
	for _, x := range records {
		if !safeCommandID(x.ID) {
			continue
		}
		fmt.Fprintf(&in, "%s\t%-10s %-20s %s\n", x.ID, sanitizeDisplay(x.Kind), sanitizeDisplay(x.ID), sanitizeDisplay(x.Summary))
	}
	cmd.Stdin = strings.NewReader(in.String())
	cmd.Stderr = r.stderr
	var out bytesBuffer
	cmd.Stdout = &out
	e = cmd.Run()
	if e != nil {
		if ee := new(exec.ExitError); errors.As(e, &ee) && (ee.ExitCode() == 1 || ee.ExitCode() == 130) {
			return nil
		}
		return fmt.Errorf("fzf: %w", e)
	}
	line := strings.TrimSpace(out.String())
	id, _, _ := strings.Cut(line, "\t")
	if id == "" {
		return nil
	}
	if selected, ok := recordByID(records, id); ok {
		return r.display(ctx, selected, false)
	}
	return nil
}

type bytesBuffer struct{ b strings.Builder }

func (b *bytesBuffer) Write(p []byte) (int, error) { return b.b.Write(p) }
func (b *bytesBuffer) String() string              { return b.b.String() }
func shellQuote(s string) string                   { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func contains(a []string, x string) bool {
	for _, v := range a {
		if v == x {
			return true
		}
	}
	return false
}
func sanitizeTSV(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
}
func sanitizeDisplay(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) {
					c := s[i]
					i++
					if c >= '@' && c <= '~' {
						break
					}
				}
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r < ' ' || r == 0x7f {
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
func recordByID(records []Record, id string) (Record, bool) {
	for _, r := range records {
		if r.ID == id {
			return r, true
		}
	}
	return Record{}, false
}
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, e := f.Stat()
	return e == nil && st.Mode()&os.ModeCharDevice != 0
}

func (r *runtime) topicCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: wtf topic <new|edit> [title|query]")
	}
	switch args[0] {
	case "new":
		if len(args) < 2 {
			return errors.New("usage: wtf topic new <title>")
		}
		return r.newTopic(ctx, strings.Join(args[1:], " "))
	case "edit":
		return r.editTopic(ctx, strings.Join(args[1:], " "))
	default:
		return fmt.Errorf("unknown topic command %q", args[0])
	}
}
func (r *runtime) newTopic(ctx context.Context, title string) error {
	id := content.Slug(title)
	if id == "" {
		return errors.New("title does not produce a valid topic id")
	}
	for _, existing := range r.records {
		if existing.Topic != nil && existing.ID == id {
			return fmt.Errorf("topic id %q already exists at %s", id, existing.Topic.Sources[len(existing.Topic.Sources)-1].Path)
		}
	}
	path := filepath.Join(r.cfg.Topics.WriteDir, id+".md")
	if _, e := os.Stat(path); e == nil {
		return fmt.Errorf("topic %q already exists at %s", id, path)
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	template := fmt.Sprintf("---\nschema: 1\nid: %s\nkind: topic\ntitle: %s\nsummary: \"\"\n---\n\n# %s\n", id, title, title)
	if e := os.WriteFile(path, []byte(template), 0600); e != nil {
		return e
	}
	return r.editFile(ctx, path, id)
}
func (r *runtime) editTopic(ctx context.Context, q string) error {
	var topics []Record
	for _, x := range r.records {
		if x.Topic != nil {
			topics = append(topics, x)
		}
	}
	if q == "" {
		return r.topicPicker(ctx, topics, "")
	}
	m := matchRecords(topics, q)
	if len(m) == 0 {
		return fmt.Errorf("no authored topic matches %q", q)
	}
	if len(m) > 1 {
		return r.topicPicker(ctx, m, q)
	}
	return r.editFile(ctx, m[0].Topic.Sources[len(m[0].Topic.Sources)-1].Path, m[0].ID)
}
func (r *runtime) topicPicker(ctx context.Context, topics []Record, query string) error {
	fzf, e := exec.LookPath("fzf")
	if e != nil {
		return errors.New("fzf is required for topic selection; pass an exact topic id or install fzf")
	}
	args := []string{"--delimiter=\t", "--with-nth=2", "--layout=reverse", "--prompt=wtf topic ▸ "}
	if query != "" {
		args = append(args, "--query", query)
	}
	cmd := exec.CommandContext(ctx, fzf, args...)
	var in strings.Builder
	for _, x := range topics {
		if !safeCommandID(x.ID) {
			continue
		}
		fmt.Fprintf(&in, "%s\t%-20s %s\n", x.ID, sanitizeDisplay(x.ID), sanitizeDisplay(x.Title))
	}
	cmd.Stdin = strings.NewReader(in.String())
	var out bytesBuffer
	cmd.Stdout = &out
	cmd.Stderr = r.stderr
	if e := cmd.Run(); e != nil {
		if ee := new(exec.ExitError); errors.As(e, &ee) && (ee.ExitCode() == 1 || ee.ExitCode() == 130) {
			return nil
		}
		return fmt.Errorf("fzf: %w", e)
	}
	id, _, _ := strings.Cut(strings.TrimSpace(out.String()), "\t")
	if selected, ok := recordByID(topics, id); ok {
		return r.editFile(ctx, selected.Topic.Sources[len(selected.Topic.Sources)-1].Path, selected.ID)
	}
	return nil
}
func (r *runtime) editFile(ctx context.Context, path, expectedID string) error {
	before, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("EDITOR is not set; topic preserved at %s", path)
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return errors.New("EDITOR is empty")
	}
	p, e := exec.LookPath(parts[0])
	if e != nil {
		return fmt.Errorf("editor %q not found", parts[0])
	}
	cmd := exec.CommandContext(ctx, p, append(parts[1:], path)...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("editor failed; topic preserved at %s: %w", path, e)
	}
	after, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if sha256.Sum256(before) == sha256.Sum256(after) {
		return nil
	}
	entries, e := content.ParseFile(path, true)
	if e != nil {
		return fmt.Errorf("saved topic is invalid (file preserved): %w", e)
	}
	if len(entries) != 1 {
		return errors.New("saved topic did not contain one topic")
	}
	if e := content.Validate(entries[0]); e != nil || entries[0].Kind != "topic" {
		if e == nil {
			e = errors.New("kind must be topic")
		}
		return fmt.Errorf("saved topic is invalid (file preserved): %w", e)
	}
	if entries[0].ID != expectedID {
		return fmt.Errorf("saved topic id changed from %q to %q (file preserved; hook not run)", expectedID, entries[0].ID)
	}
	if filepath.Clean(path) != path || filepath.Base(path) != expectedID+".md" {
		return fmt.Errorf("topic path no longer matches id %q (file preserved; hook not run)", expectedID)
	}
	if len(r.cfg.Topics.PostWrite) > 0 {
		argv := make([]string, len(r.cfg.Topics.PostWrite))
		for i, a := range r.cfg.Topics.PostWrite {
			argv[i] = strings.ReplaceAll(a, "{path}", path)
		}
		p, e := exec.LookPath(argv[0])
		if e != nil {
			return fmt.Errorf("post_write command failed (file preserved): %s: %w", strings.Join(argv, " "), e)
		}
		c := exec.CommandContext(ctx, p, argv[1:]...)
		c.Stdin = nil
		c.Stdout = r.stdout
		c.Stderr = r.stderr
		if e := c.Run(); e != nil {
			return fmt.Errorf("post_write command failed (file preserved): %s: %w", strings.Join(argv, " "), e)
		}
	}
	return nil
}

func initCommand(args []string, w io.Writer) error {
	if len(args) != 1 || args[0] != "zsh" {
		return errors.New("usage: wtf init zsh")
	}
	fmt.Fprint(w, zshInit)
	return nil
}
func completion(args []string, w io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: wtf completion <zsh|bash|fish>")
	}
	switch args[0] {
	case "zsh":
		fmt.Fprint(w, zshCompletion)
	case "bash":
		fmt.Fprint(w, bashCompletion)
	case "fish":
		fmt.Fprint(w, fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q", args[0])
	}
	return nil
}

const usage = `wtf — discover installed commands and read composed documentation

Usage:
  wtf                         Open the interactive picker
  wtf <query>                 Resolve a document or prefilter the picker
  wtf list [-a|--all] [--json]
  wtf topic new <title>
  wtf topic edit [query]
  wtf cache status [--json]
  wtf cache refresh
  wtf init zsh
  wtf completion <zsh|bash|fish>
  wtf version
`
const zshInit = `# generated by wtf init zsh
function wtf() {
  if (( $# > 0 )) && [[ "$1" != -* ]] && [[ "$1" != list && "$1" != topic && "$1" != cache && "$1" != init && "$1" != completion && "$1" != version ]]; then
    local _wtf_name="$1" _wtf_kind="" _wtf_value=""
    local -a _wtf_meta
    if (( $+functions[$_wtf_name] )); then
      _wtf_kind=function
      _wtf_value="${functions[$_wtf_name]}"
    elif (( $+aliases[$_wtf_name] )); then
      _wtf_kind=alias
      _wtf_value="${aliases[$_wtf_name]}"
      local _wtf_current="$_wtf_name" _wtf_target=""
      local -A _wtf_seen
      while (( $+aliases[$_wtf_current] )); do
        if [[ -n "${_wtf_seen[$_wtf_current]}" ]]; then
          _wtf_meta+=(--shell-cycle)
          break
        fi
        _wtf_seen[$_wtf_current]=1
        local _wtf_expansion="${aliases[$_wtf_current]}"
        local -a _wtf_words
        _wtf_words=( ${(z)_wtf_expansion} )
        if (( ${#_wtf_words} == 0 )); then
          _wtf_meta+=(--shell-complex)
          break
        fi
        local _wtf_word
        for _wtf_word in "${_wtf_words[@]}"; do
          if [[ "$_wtf_word" == '|' || "$_wtf_word" == '||' || "$_wtf_word" == '&' || "$_wtf_word" == '&&' || "$_wtf_word" == ';' || "$_wtf_word" == *'>'* || "$_wtf_word" == *'<'* || "$_wtf_word" == *'$'* || "$_wtf_word" == *$'\x60'* || "$_wtf_word" == *'('* || "$_wtf_word" == *')'* ]]; then
            _wtf_meta+=(--shell-complex)
            break 2
          fi
        done
        _wtf_target="${_wtf_words[1]}"
        _wtf_current="$_wtf_target"
      done
      if (( ${#_wtf_meta} == 0 )) && [[ -n "$_wtf_target" ]]; then
        _wtf_meta+=(--shell-target "$_wtf_target")
      fi
    fi
    if [[ -n "$_wtf_kind" ]]; then
      command wtf --shell-kind "$_wtf_kind" "$_wtf_name" "$_wtf_value" "${_wtf_meta[@]}" "$@"
      return $?
    fi
  fi
  command wtf "$@"
}
`
const zshCompletion = `#compdef wtf
_wtf() {
  local context state line
  typeset -A opt_args
  _arguments -C \
    '1:command:->command' \
    '*::argument:->arguments'
  case $state in
    command)
      _values 'command' \
        'list[list commands]' 'topic[author topics]' 'cache[inspect package cache]' \
        'init[emit shell integration]' 'completion[emit completions]' 'version[print version]' \
        ${(f)"$(command wtf list -a 2>/dev/null | cut -f2)"}
      ;;
    arguments)
      case $words[2] in
        list) _arguments '(-a --all)'{-a,--all}'[include unavailable annotations]' '--json[emit JSON]' ;;
        topic)
          if (( CURRENT == 3 )); then _values 'topic command' 'new[create topic]' 'edit[edit topic]'
          elif [[ $words[3] == edit ]]; then _values 'topic' ${(f)"$(command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}')"}
          else _message 'topic title'; fi ;;
        cache)
          if (( CURRENT == 3 )); then _values 'cache command' 'status[show cache status]' 'refresh[refresh cache]'
          elif [[ $words[3] == status ]]; then _arguments '--json[emit JSON]'; fi ;;
        init) _values 'shell' zsh ;;
        completion) _values 'shell' zsh bash fish ;;
        *) _values 'command' ${(f)"$(command wtf list -a 2>/dev/null | cut -f2)"} ;;
      esac
      ;;
  esac
}
_wtf "$@"
`
const bashCompletion = `_wtf_complete() {
  local cur prev
  cur=${COMP_WORDS[COMP_CWORD]}
  prev=${COMP_WORDS[COMP_CWORD-1]}
  if (( COMP_CWORD == 1 )); then
    COMPREPLY=( $(compgen -W "list topic cache init completion version $(command wtf list -a 2>/dev/null | cut -f2)" -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == list ]]; then
    COMPREPLY=( $(compgen -W '-a --all --json' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == topic && $COMP_CWORD == 2 ]]; then
    COMPREPLY=( $(compgen -W 'new edit' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == cache && $COMP_CWORD == 2 ]]; then
    COMPREPLY=( $(compgen -W 'status refresh' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == cache && ${COMP_WORDS[2]} == status ]]; then
    COMPREPLY=( $(compgen -W '--json' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == init ]]; then
    COMPREPLY=( $(compgen -W 'zsh' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == completion ]]; then
    COMPREPLY=( $(compgen -W 'zsh bash fish' -- "$cur") )
  elif [[ ${COMP_WORDS[1]} == topic && ${COMP_WORDS[2]} == edit ]]; then
    COMPREPLY=( $(compgen -W "$(command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}')" -- "$cur") )
  fi
}
complete -F _wtf_complete wtf
`
const fishCompletion = `complete -c wtf -f
function __wtf_topic_ids
  command wtf list -a 2>/dev/null | awk -F '\t' '$1 == "topic" {print $2}'
end
complete -c wtf -n '__fish_use_subcommand' -a list -d 'List commands'
complete -c wtf -n '__fish_use_subcommand' -a topic -d 'Author topics'
complete -c wtf -n '__fish_use_subcommand' -a cache -d 'Inspect package cache'
complete -c wtf -n '__fish_use_subcommand' -a init -d 'Emit shell integration'
complete -c wtf -n '__fish_use_subcommand' -a completion -d 'Emit completions'
complete -c wtf -n '__fish_use_subcommand' -a version -d 'Print version'
complete -c wtf -n '__fish_seen_subcommand_from list' -s a -l all -d 'Include unavailable annotations'
complete -c wtf -n '__fish_seen_subcommand_from list' -l json -d 'Emit JSON'
complete -c wtf -n '__fish_seen_subcommand_from topic; and not __fish_seen_subcommand_from new edit' -a 'new edit'
complete -c wtf -n '__fish_seen_subcommand_from topic; and __fish_seen_subcommand_from edit' -a '(__wtf_topic_ids)'
complete -c wtf -n '__fish_seen_subcommand_from cache; and not __fish_seen_subcommand_from status refresh' -a 'status refresh'
complete -c wtf -n '__fish_seen_subcommand_from cache; and __fish_seen_subcommand_from status' -l json -d 'Emit JSON'
complete -c wtf -n '__fish_seen_subcommand_from init' -a zsh
complete -c wtf -n '__fish_seen_subcommand_from completion' -a 'zsh bash fish'
`
