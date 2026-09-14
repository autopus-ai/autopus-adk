package config

import "fmt"

// DefaultCoverageThreshold is the coverage floor a project inherits when it
// declares no `workflow.coverage_threshold`. It is the single source of the
// number: defaults, the missing-key backfill, and the CLI fallback all read it
// so a project never faces two different "default" floors.
const DefaultCoverageThreshold = 85

// WorkflowConf holds the workflow configuration settings.
type WorkflowConf struct {
	CoverageThreshold int `yaml:"coverage_threshold,omitempty"`
}

// Validate checks that the workflow configuration is valid.
// An out-of-range coverage threshold (outside 0..100) is rejected.
func (w WorkflowConf) Validate() error {
	if w.CoverageThreshold < 0 || w.CoverageThreshold > 100 {
		return fmt.Errorf("workflow: coverage_threshold %d must be between 0 and 100", w.CoverageThreshold)
	}
	return nil
}
