package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contractTypes(t *testing.T) []string {
	files, _ := filepath.Glob("../../knowledge/contracts/*.yaml")
	var types []string
	for _, f := range files {
		types = append(types, strings.TrimSuffix(filepath.Base(f), ".yaml"))
	}
	return types
}

func TestShippedWorkflowsLoad(t *testing.T) {
	wfs, err := Load("../../workflows", contractTypes(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"feature", "feature-ui", "quick-fix", "change", "e2e-tests"} {
		if _, ok := wfs[name]; !ok {
			t.Errorf("workflow %s missing", name)
		}
	}
	if !wfs["quick-fix"].StartsWithCode() || wfs["feature"].StartsWithCode() {
		t.Error("StartsWithCode is wrong")
	}
	if s, _, _ := wfs["feature"].Step("plan"); !s.PlanGate() {
		t.Error("feature plan should be gated")
	}
}

func TestInvalidWorkflows(t *testing.T) {
	for name, body := range map[string]string{
		"unknown kind":      "name: x\nsteps:\n  - {name: a, kind: magic, role: r}\n",
		"forward reference": "name: x\nsteps:\n  - {name: a, kind: document, doc_type: prd, role: r, inputs: [b]}\n  - {name: b, kind: document, doc_type: prd, role: r}\n",
		"plan without code": "name: x\nsteps:\n  - {name: p, kind: plan, doc_type: implementation-plan, role: r}\n",
		"unknown doc type":  "name: x\nsteps:\n  - {name: a, kind: document, doc_type: memo, role: r}\n",
		"revise without cr": "name: x\nsteps:\n  - {name: a, kind: document, doc_type: prd, role: r}\n  - {name: b, kind: revise, role: r, inputs: [a]}\n",
		"unknown field":     "name: x\nsteps:\n  - {name: a, kind: code, role: r, reviewer: true}\n",
	} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(body), 0o644)
		if _, err := Load(dir, contractTypes(t)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
