package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// persistUpdateConfigMigrations writes the config migrations that auto update
// applies before it renders platform files. A migration saves through
// config.Save, whose schema has no retired orchestra key. A file that needs
// none still loses its retired keys (SPEC-PANERM-001 REQ-10), and its retired
// plan and brainstorm command defaults, but through a raw rewrite that keeps
// comments, env placeholders, and reserved blocks, which config.Save would
// drop. Either way the removed keys are named once.
func persistUpdateConfigMigrations(out io.Writer, dir string, cfg *config.HarnessConfig, designConfigMissing bool) error {
	// Before the orchestra migration, which would append new providers to
	// the retired entries and make them look edited.
	stale := config.PruneRetiredOrchestraCommands(cfg)
	orchestraMigrated, err := config.MigrateOrchestraConfig(cfg)
	if err != nil {
		return fmt.Errorf("orchestra 마이그레이션 실패: %w", err)
	}
	var removed []string
	if orchestraMigrated || (designConfigMissing && cfg.Design.Enabled) {
		removed = append(retiredConfigKeysInFile(dir), stale...)
		if err := config.Save(dir, cfg); err != nil {
			return fmt.Errorf("마이그레이션 설정 저장 실패: %w", err)
		}
	} else if removed, err = pruneConfigFileKeys(dir, stale); err != nil {
		return fmt.Errorf("retired orchestra key cleanup failed: %w", err)
	}
	writeRemovedConfigKeys(out, removed, stale)
	return nil
}

// writeRemovedConfigKeys names the retired orchestra keys and, on a line of
// its own, the retired command defaults a run removed.
func writeRemovedConfigKeys(out io.Writer, removed, stale []string) {
	var retired, commands []string
	for _, path := range removed {
		if slices.Contains(stale, path) {
			commands = append(commands, path)
		} else {
			retired = append(retired, path)
		}
	}
	if len(retired) > 0 {
		fmt.Fprintf(out, "  - removed retired orchestra keys from autopus.yaml: %s\n", terminalSafe(strings.Join(retired, ", ")))
	}
	if len(commands) > 0 {
		fmt.Fprintf(out, "  - removed retired orchestra command defaults from autopus.yaml: %s\n", strings.Join(commands, ", "))
	}
}
