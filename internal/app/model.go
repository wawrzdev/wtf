package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/wawrzdev/wtf/internal/content"
	"github.com/wawrzdev/wtf/internal/inventory"
)

type Record struct {
	ID            string              `json:"id"`
	Kind          string              `json:"kind"`
	Title         string              `json:"title,omitempty"`
	Summary       string              `json:"summary,omitempty"`
	Parent        string              `json:"parent,omitempty"`
	Members       []string            `json:"members,omitempty"`
	Available     bool                `json:"available"`
	Path          string              `json:"path,omitempty"`
	ShadowedPaths []string            `json:"shadowed_paths,omitempty"`
	Version       string              `json:"version,omitempty"`
	Packages      []inventory.Package `json:"packages,omitempty"`
	Annotation    *content.Entry      `json:"annotation,omitempty"`
	Topic         *content.Entry      `json:"topic,omitempty"`
	Shell         *ShellResolution    `json:"shell,omitempty"`
	HasTLDR       bool                `json:"has_tldr"`
}
type ShellResolution struct {
	Kind      string     `json:"kind"`
	Name      string     `json:"name"`
	Expansion string     `json:"expansion,omitempty"`
	Target    string     `json:"target,omitempty"`
	Complex   bool       `json:"complex,omitempty"`
	Cycle     bool       `json:"cycle,omitempty"`
	Hops      []ShellHop `json:"hops,omitempty"`
}
type ShellHop struct {
	Name      string `json:"name"`
	Expansion string `json:"expansion"`
}

func buildRecords(anns, topics map[string]content.Entry, pkgs []inventory.Package, includeUnavailable bool) []Record {
	m := map[string]*Record{}
	get := func(id string) *Record {
		if m[id] == nil {
			m[id] = &Record{ID: id, Kind: "command"}
		}
		return m[id]
	}
	for _, p := range pkgs {
		for _, cmd := range p.Commands {
			if !safeCommandID(cmd) {
				continue
			}
			r := get(cmd)
			r.Packages = append(r.Packages, p)
			if r.Version == "" {
				r.Version = p.Version
			}
		}
	}
	for id, a := range anns {
		r := get(id)
		copy := a
		r.Annotation = &copy
		r.Summary = a.Summary
		r.Parent = a.Parent
		if a.Kind != "tool" && a.Kind != "" {
			r.Kind = a.Kind
		}
	}
	for id, t := range topics {
		r := get(id)
		copy := t
		r.Topic = &copy
		if r.Summary == "" {
			r.Summary = t.Summary
		}
		if t.Title != "" {
			r.Title = t.Title
		}
		if r.Annotation == nil && len(r.Packages) == 0 {
			r.Kind = "topic"
		}
	}
	var out []Record
	for _, r := range m {
		paths := findAll(r.ID)
		if len(paths) > 0 {
			r.Available = true
			r.Path = paths[0]
			if len(paths) > 1 {
				r.ShadowedPaths = paths[1:]
			}
		}
		if r.Available || includeUnavailable || r.Kind == "topic" || r.Kind == "group" {
			out = append(out, *r)
		}
	}
	sortRecords(out)
	return out
}

// shellDefinition reports an annotated alias or function, which lives in the shell
// rather than on PATH and so cannot be checked by the binary.
func shellDefinition(x Record) bool {
	return !x.Available && (x.Kind == "alias" || x.Kind == "func")
}

// visibleByDefault decides which records list and the picker show without --all.
func visibleByDefault(x Record) bool {
	return x.Available || x.Kind == "topic" || x.Kind == "group" || x.HasTLDR || shellDefinition(x)
}

// kindOrder lists shell definitions first, then commands, groups of commands and topics.
var kindOrder = map[string]int{"alias": 0, "func": 1, "command": 2, "group": 3, "topic": 4}

func kindRank(kind string) int {
	if n, ok := kindOrder[kind]; ok {
		return n
	}
	return len(kindOrder)
}

// sortRecords orders records by kind, then ID, for both list output and the picker.
func sortRecords(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool {
		if a, b := kindRank(rs[i].Kind), kindRank(rs[j].Kind); a != b {
			return a < b
		}
		return rs[i].ID < rs[j].ID
	})
}

func safeCommandID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._+-", r)) {
			return false
		}
	}
	return true
}

func mergeTLDR(records []Record) ([]Record, error) {
	seen := map[string]int{}
	for i := range records {
		seen[records[i].ID] = i
	}
	ids, err := localTLDRIDs()
	for _, id := range ids {
		if i, ok := seen[id]; ok {
			records[i].HasTLDR = true
			continue
		}
		seen[id] = len(records)
		records = append(records, Record{ID: id, Kind: "command", HasTLDR: true})
	}
	sortRecords(records)
	return records, err
}

func enrichAnnotatedSystemPackages(ctx context.Context, records []Record) []Record {
	for i := range records {
		r := &records[i]
		if r.Annotation == nil || !r.Available {
			continue
		}
		known := map[string]bool{}
		for _, p := range r.Packages {
			known[p.Provider+"\x00"+p.Name] = true
		}
		for _, p := range inventory.ExactSystem(ctx, r.ID, r.Path) {
			if !known[p.Provider+"\x00"+p.Name] {
				r.Packages = append(r.Packages, p)
			}
			if r.Version == "" {
				r.Version = p.Version
			}
		}
	}
	return records
}
func findAll(name string) []string {
	if strings.ContainsRune(name, '/') {
		return []string{name}
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range strings.Split(envPath(), stringPathSep()) {
		p := joinPath(d, name)
		if executable(p) && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	return out
}

func marshalStable(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

// linkPackages makes a package that provides several commands browsable as a unit.
// Each extra command gets the package's main command as its parent (node → npm). A
// package with no command of its own name (libpq, awscli) becomes a group record that
// its commands belong to. Declared annotation parents are kept. Members are then
// derived from parents, documented members first.
func linkPackages(records []Record) []Record {
	index := map[string]int{}
	for i, x := range records {
		index[x.ID] = i
	}
	byPackage := map[string][]int{}
	var order []string
	for i, x := range records {
		if x.Kind != "command" || len(x.Packages) == 0 {
			continue
		}
		k := x.Packages[0].Provider + ":" + x.Packages[0].Name
		if byPackage[k] == nil {
			order = append(order, k)
		}
		byPackage[k] = append(byPackage[k], i)
	}
	for _, k := range order {
		group := byPackage[k]
		if len(group) < 2 {
			continue
		}
		name := packageShortName(records[group[0]].Packages[0].Name)
		if _, ok := index[name]; !ok {
			g := Record{ID: name, Kind: "group", Available: true}
			for _, i := range group {
				if records[i].Summary != "" && g.Summary == "" {
					g.Summary = records[i].Summary
				}
			}
			index[name] = len(records)
			records = append(records, g)
		}
		for _, i := range group {
			if records[i].ID != name && records[i].Parent == "" {
				records[i].Parent = name
			}
		}
	}
	for i := range records {
		records[i].Members = nil
	}
	for _, x := range records {
		if p, ok := index[x.Parent]; ok && x.Parent != "" {
			records[p].Members = append(records[p].Members, x.ID)
		}
	}
	for i := range records {
		m := records[i].Members
		sort.SliceStable(m, func(a, b int) bool {
			da, db := records[index[m[a]]].Summary != "", records[index[m[b]]].Summary != ""
			if da != db {
				return da
			}
			return m[a] < m[b]
		})
	}
	// An undocumented parent (httpie, lima) borrows its first documented member's
	// summary so its row still says what it is.
	for i := range records {
		if records[i].Summary == "" && len(records[i].Members) > 0 {
			records[i].Summary = records[index[records[i].Members[0]]].Summary
		}
	}
	sortRecords(records)
	return records
}

// packageShortName drops tap prefixes and version suffixes: "wawrzdev/packages/wtf" → "wtf".
func packageShortName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.Index(name, "@"); i > 0 {
		name = name[:i]
	}
	return name
}
