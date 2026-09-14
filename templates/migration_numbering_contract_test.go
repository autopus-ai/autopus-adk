package templates_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

func TestMigrationNumberingGuidanceContracts(t *testing.T) {
	t.Parallel()

	root := templateRoot()
	e := tmpl.New()
	cfg := config.DefaultFullConfig("migration-project")
	files := map[string][]string{
		filepath.Join(root, "..", "content", "profiles", "executor", "go.md"): {
			"Project-Scoped Directory Resolution",
			"serialized numbering lane",
			"fmt.Sprintf(\"%06d\", nextNum)",
			"same full stem",
			"missing same-stem",
		},
		filepath.Join(root, "..", "content", "skills", "database.md"): {
			"000001_create_users.up.sql",
			"프로젝트별 넘버링 규칙",
			"동일 migration directory",
			"same stem",
			"missing same-stem counterpart",
		},
		filepath.Join(root, "codex", "skills", "database.md.tmpl"): {
			"000001_create_users.up.sql",
			"프로젝트별 넘버링 규칙",
			"동일 migration directory",
			"same stem",
			"missing same-stem counterpart",
		},
		filepath.Join(root, "gemini", "skills", "database", "SKILL.md.tmpl"): {
			"000001_create_users.up.sql",
			"프로젝트별 넘버링 규칙",
			"동일 migration directory",
			"same stem",
			"missing same-stem counterpart",
		},
		filepath.Join(root, "..", "content", "agents", "validator.md"): {
			"directory-scoped numbering",
			"--diff-filter=ACMRD",
			"same full stem",
			"New pair ordering",
			"missing same-stem counterpart",
			"parallel number reservation",
		},
		filepath.Join(root, "codex", "agents", "validator.toml.tmpl"): {
			"directory-scoped numbering",
			"--diff-filter=ACMRD",
			"same full stem",
			"New pair ordering",
			"missing same-stem counterpart",
			"parallel number reservation",
		},
		filepath.Join(root, "gemini", "agents", "validator.md.tmpl"): {
			"directory-scoped numbering",
			"--diff-filter=ACMRD",
			"same full stem",
			"New pair ordering",
			"missing same-stem counterpart",
			"parallel number reservation",
		},
		filepath.Join(root, "..", "content", "skills", "agent-pipeline.md"): {
			"Migration numbering rule",
			"same owning repo and migration directory",
		},
		filepath.Join(root, "gemini", "skills", "agent-pipeline", "SKILL.md.tmpl"): {
			"Migration numbering rule",
			"same owning repo and migration directory",
		},
		filepath.Join(root, "claude", "commands", "auto-router.md.tmpl"): {
			"Migration directory serialization",
			"no migration-directory conflict",
		},
		filepath.Join(root, "gemini", "commands", "auto-router.md.tmpl"): {
			"Migration directory serialization",
			"no migration-directory conflict",
		},
		filepath.Join(root, "..", "content", "skills", "worktree-isolation.md"): {
			"Migration numbering lane",
			"same migration numbering lane",
		},
		filepath.Join(root, "gemini", "skills", "worktree-isolation", "SKILL.md.tmpl"): {
			"Migration numbering lane",
			"same migration numbering lane",
		},
		filepath.Join(root, "..", "pkg", "adapter", "codex", "codex_extended_skill_rewrites_worktree.go"): {
			"same owning repo and migration directory",
			"migration numbering lane",
		},
	}

	for path, expected := range files {
		path, expected := path, expected
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			content, err := semanticContractSurface(e, path, cfg)
			require.NoError(t, err)
			for _, phrase := range expected {
				assert.Contains(t, content, phrase)
			}
		})
	}
}
