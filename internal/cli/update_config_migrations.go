package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// persistUpdateConfigMigrations writes the config migrations that auto update
// applies before it renders platform files. A migration saves through
// config.Save, whose schema has no retired orchestra key. A file that needs
// none still loses its retired keys (SPEC-PANERM-001 REQ-10), but through a
// raw rewrite that keeps comments, env placeholders, and reserved blocks,
// which config.Save would drop. Either way the removed keys are named once.
func persistUpdateConfigMigrations(out io.Writer, dir string, cfg *config.HarnessConfig, designConfigMissing bool) error {
	orchestraMigrated, err := config.MigrateOrchestraConfig(cfg)
	if err != nil {
		return fmt.Errorf("orchestra 마이그레이션 실패: %w", err)
	}
	var retired []string
	if orchestraMigrated || (designConfigMissing && cfg.Design.Enabled) {
		retired = retiredConfigKeysInFile(dir)
		if err := config.Save(dir, cfg); err != nil {
			return fmt.Errorf("마이그레이션 설정 저장 실패: %w", err)
		}
	} else if retired, err = pruneRetiredConfigFile(dir); err != nil {
		return fmt.Errorf("retired orchestra key cleanup failed: %w", err)
	}
	if len(retired) > 0 {
		fmt.Fprintf(out, "  - removed retired orchestra keys from autopus.yaml: %s\n", terminalSafe(strings.Join(retired, ", ")))
	}
	return nil
}
