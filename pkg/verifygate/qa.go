package verifygate

import (
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// qaStep runs the project's exec test cases after its build and tests, so a
// behaviour a QA run once confirmed is checked again on every product change.
// It is planned only when another step was: those are what a source change
// asks for, and an edited README or scenario file is not a product change.
// Exit 2, a case that could not run, does not block: it is the case's or the
// machine's problem, not the change's.
func qaStep(root string) (Step, bool) {
	if !hasExecCases(root) {
		return Step{}, false
	}
	return Step{
		Name:     "qa exec --all",
		Argv:     []string{autoProgram(), "qa", "exec", "--all", "--project-dir", root},
		Dir:      root,
		PassExit: []int{2},
	}, true
}

func hasExecCases(root string) bool {
	paths, _ := filepath.Glob(filepath.Join(root, testscenario.DirRel, "*.yaml"))
	for _, path := range paths {
		doc, err := testscenario.LoadFile(path)
		if err != nil {
			continue
		}
		for _, c := range doc.Cases {
			if c.Automation.Type == testscenario.AutomationExec && c.Automation.Exec != nil {
				return true
			}
		}
	}
	return false
}

// autoProgram is the running auto binary, which knows `qa exec --all`; an
// older auto earlier on PATH may not. A test binary falls back to the name.
func autoProgram() string {
	if self, err := os.Executable(); err == nil && filepath.Base(self) == "auto" {
		return self
	}
	return "auto"
}
