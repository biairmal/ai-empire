// Package workflow loads workflow definitions (spec §7): ordered steps that turn
// a request into documents, a plan, and code, with human gates in between.
package workflow

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// Step kinds.
const (
	Document = "document" // one document (plus allowed extras) → approval gate
	Plan     = "plan"     // implementation plan → optional gate → code tasks
	Revise   = "revise"   // new version of each document a change request affects → gate
	Code     = "code"     // developer tasks (from the plan, or one task for the request)
)

type Step struct {
	Name       string              `yaml:"name"`
	Kind       string              `yaml:"kind"`
	DocType    string              `yaml:"doc_type"`
	Role       string              `yaml:"role"`
	Inputs     []string            `yaml:"inputs"`      // earlier steps whose documents are given as context
	ExtraTypes []string            `yaml:"extra_types"` // additional document types the step may produce
	Relations  map[string][]string `yaml:"relations"`   // relationship → earlier steps whose documents it points to
	Gate       *bool               `yaml:"gate"`        // plan steps only; documents always follow their contract
	Review     bool                `yaml:"review"`      // code steps: AI review before the merge gate
}

// PlanGate reports whether a plan step waits for human approval (default yes).
func (s Step) PlanGate() bool { return s.Gate == nil || *s.Gate }

type Workflow struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Steps       []Step `yaml:"steps"`
}

// Step returns the named step.
func (w Workflow) Step(name string) (Step, int, bool) {
	for i, s := range w.Steps {
		if s.Name == name {
			return s, i, true
		}
	}
	return Step{}, -1, false
}

// StartsWithCode reports whether the workflow creates code without a plan,
// which means the request itself must name a repository.
func (w Workflow) StartsWithCode() bool {
	for _, s := range w.Steps {
		switch s.Kind {
		case Plan:
			return false
		case Code:
			return true
		}
	}
	return false
}

// Load reads every *.yaml workflow in dir. docTypes lists the known document types.
func Load(dir string, docTypes []string) (map[string]Workflow, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := map[string]Workflow{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var w Workflow
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&w); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := w.check(docTypes); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if _, dup := out[w.Name]; dup {
			return nil, fmt.Errorf("%s: duplicate workflow name %q", f, w.Name)
		}
		out[w.Name] = w
	}
	return out, nil
}

func (w Workflow) check(docTypes []string) error {
	if w.Name == "" || len(w.Steps) == 0 {
		return fmt.Errorf("a workflow needs a name and at least one step")
	}
	seen := map[string]Step{}
	for i, s := range w.Steps {
		if s.Name == "" || s.Role == "" {
			return fmt.Errorf("step %d needs a name and a role", i+1)
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("duplicate step %q", s.Name)
		}
		switch s.Kind {
		case Document:
			if !slices.Contains(docTypes, s.DocType) {
				return fmt.Errorf("step %s: unknown doc_type %q", s.Name, s.DocType)
			}
		case Plan:
			if s.DocType != "implementation-plan" {
				return fmt.Errorf("step %s: plan steps produce doc_type implementation-plan", s.Name)
			}
			if i+1 >= len(w.Steps) || w.Steps[i+1].Kind != Code {
				return fmt.Errorf("step %s: a plan step must be followed by a code step", s.Name)
			}
		case Revise:
			if !slices.ContainsFunc(s.Inputs, func(in string) bool { return seen[in].DocType == "change-request" }) {
				return fmt.Errorf("step %s: a revise step needs a change-request step in its inputs", s.Name)
			}
		case Code:
		default:
			return fmt.Errorf("step %s: unknown kind %q", s.Name, s.Kind)
		}
		for _, t := range s.ExtraTypes {
			if !slices.Contains(docTypes, t) {
				return fmt.Errorf("step %s: unknown extra type %q", s.Name, t)
			}
		}
		refs := slices.Clone(s.Inputs)
		for _, steps := range s.Relations {
			refs = append(refs, steps...)
		}
		for _, in := range refs {
			if _, ok := seen[in]; !ok {
				return fmt.Errorf("step %s refers to %q, which is not an earlier step", s.Name, in)
			}
		}
		seen[s.Name] = s
	}
	return nil
}
