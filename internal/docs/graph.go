package docs

import "slices"

// Context expansion (spec §22 "Context Resolver"): starting from the documents
// a task names, follow the knowledge graph to the other project documents the
// agent needs — the requirements they satisfy, the decisions they depend on,
// the API/database specs that implement a design, and the test plans.

// Outgoing relationships worth following: this document relies on the target.
var contextOut = []string{"satisfies", "implements", "depends_on", "derived_from", "affects", "tested_by"}

// Incoming relationships worth following: the source details this document.
var contextIn = []string{"implements"}

// ExpandContext returns start plus every document reachable within depth hops,
// limited to the same scope as the starting documents (other scopes are loaded
// wholesale by the context resolver already). Superseded and rejected documents
// are skipped. The result keeps start's order, then discovery order.
func (r *Repo) ExpandContext(start []*Doc, client string, depth int) []*Doc {
	seen := map[Key]bool{}
	var out []*Doc
	add := func(d *Doc) bool {
		if d == nil || seen[d.Key()] || d.Status == Superseded || d.Status == Rejected {
			return false
		}
		seen[d.Key()] = true
		out = append(out, d)
		return true
	}
	frontier := []*Doc{}
	for _, d := range start {
		if add(d) {
			frontier = append(frontier, d)
		}
	}
	for ; depth > 0 && len(frontier) > 0; depth-- {
		var next []*Doc
		for _, d := range frontier {
			for _, rel := range contextOut {
				for _, ref := range d.Rels[rel] {
					if t, err := r.Resolve(d, client, ref); err == nil && t.Scope == d.Scope && add(t) {
						next = append(next, t)
					}
				}
			}
			for _, other := range r.Docs {
				if other.Scope != d.Scope {
					continue
				}
				for _, rel := range contextIn {
					if slices.ContainsFunc(other.Rels[rel], func(ref string) bool {
						t, err := r.Resolve(other, client, ref)
						return err == nil && t.Key() == d.Key()
					}) && add(other) {
						next = append(next, other)
					}
				}
			}
		}
		frontier = next
	}
	return out
}
