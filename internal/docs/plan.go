package docs

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PlanTask is one entry of an Implementation Plan's work breakdown.
type PlanTask struct {
	Key         string   `yaml:"key" json:"key"`
	Repository  string   `yaml:"repository" json:"repository"`
	Title       string   `yaml:"title" json:"title"`
	Description string   `yaml:"description" json:"description"`
	DependsOn   []string `yaml:"depends_on" json:"depends_on"`
}

var (
	planKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	yamlFence = regexp.MustCompile("(?s)```ya?ml[ \t]*\n(.*?)\n```")
)

// SetWorkBreakdown replaces the yaml block of the "Work Breakdown" section.
func (d *Doc) SetWorkBreakdown(tasks []PlanTask) {
	out, err := yaml.Marshal(map[string][]PlanTask{"tasks": tasks})
	if err != nil {
		panic(err)
	}
	block := "```yaml\n" + strings.TrimRight(string(out), "\n") + "\n```"
	replaced := false
	d.Body = yamlFence.ReplaceAllStringFunc(d.Body, func(m string) string {
		if replaced {
			return m
		}
		replaced = true
		return block
	})
	if !replaced {
		d.Body += "\n## Work Breakdown\n\n" + block + "\n"
	}
}

// ParsePlan extracts and checks the work breakdown. repos, when non-nil, lists
// the project's repository names that tasks may target. Tasks are returned in
// an order where dependencies come first.
func ParsePlan(body string, repos []string) ([]PlanTask, error) {
	sec, ok := Sections(body)["work breakdown"]
	if !ok {
		return nil, errors.New(`missing section "## Work Breakdown"`)
	}
	blocks := yamlFence.FindAllStringSubmatch(sec, -1)
	if len(blocks) != 1 {
		return nil, fmt.Errorf("expected exactly one ```yaml block, found %d", len(blocks))
	}
	var plan struct {
		Tasks []PlanTask `yaml:"tasks"`
	}
	dec := yaml.NewDecoder(strings.NewReader(blocks[0][1]))
	dec.KnownFields(true)
	if err := dec.Decode(&plan); err != nil {
		return nil, fmt.Errorf("yaml: %v", err)
	}
	if len(plan.Tasks) == 0 {
		return nil, errors.New("no tasks")
	}

	var errs []string
	byKey := map[string]PlanTask{}
	for i, t := range plan.Tasks {
		label := fmt.Sprintf("task %d (%s)", i+1, t.Key)
		if !planKeyRe.MatchString(t.Key) {
			errs = append(errs, fmt.Sprintf("%s: key must be lowercase letters, digits and dashes", label))
		}
		if _, dup := byKey[t.Key]; dup {
			errs = append(errs, fmt.Sprintf("%s: duplicate key", label))
		}
		if strings.TrimSpace(t.Title) == "" || len(t.Title) > 72 {
			errs = append(errs, fmt.Sprintf("%s: title is required and must be at most 72 characters", label))
		}
		if strings.TrimSpace(t.Description) == "" {
			errs = append(errs, fmt.Sprintf("%s: description is required", label))
		}
		if t.Repository == "" {
			errs = append(errs, fmt.Sprintf("%s: repository is required", label))
		} else if repos != nil && !slices.Contains(repos, t.Repository) {
			errs = append(errs, fmt.Sprintf("%s: unknown repository %q (project has: %s)", label, t.Repository, strings.Join(repos, ", ")))
		}
		byKey[t.Key] = t
	}
	for _, t := range plan.Tasks {
		for _, dep := range t.DependsOn {
			if _, ok := byKey[dep]; !ok {
				errs = append(errs, fmt.Sprintf("task %s depends on unknown task %q", t.Key, dep))
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}

	// Topological order; a leftover means a cycle.
	var ordered []PlanTask
	done := map[string]bool{}
	for len(ordered) < len(plan.Tasks) {
		progress := false
		for _, t := range plan.Tasks {
			if done[t.Key] {
				continue
			}
			ready := true
			for _, dep := range t.DependsOn {
				ready = ready && done[dep]
			}
			if ready {
				ordered, done[t.Key], progress = append(ordered, t), true, true
			}
		}
		if !progress {
			return nil, errors.New("task dependencies contain a cycle")
		}
	}
	return ordered, nil
}
