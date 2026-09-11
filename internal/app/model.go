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
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Expansion string `json:"expansion,omitempty"`
	Target    string `json:"target,omitempty"`
	Complex   bool   `json:"complex,omitempty"`
	Cycle     bool   `json:"cycle,omitempty"`
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
		if r.Available || includeUnavailable || r.Kind == "topic" {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
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
