package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

const claudeManifestPath = ".autopus/" + adapterName + "-manifest.json"

func loadClaudeManifestSafely(root string) (*adapter.Manifest, error) {
	if err := adapter.RejectSymlinkComponents(root, claudeManifestPath); err != nil {
		return nil, err
	}
	return adapter.LoadManifest(root, adapterName)
}

func readClaudeCleanFile(root, path string) ([]byte, bool, error) {
	if err := adapter.RejectSymlinkComponents(root, path); err != nil {
		return nil, false, err
	}
	target, err := adapter.SafePruneFilePath(root, path)
	if err != nil || target == "" {
		return nil, false, err
	}
	data, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func claudeCleanAllowedPaths() (map[string]bool, error) {
	allowed := map[string]bool{
		".claude/statusline.sh": true, ".claude/statusline-combined.sh": true,
		".claude/statusline-user-command.txt":   true,
		".claude/workflows/route_a.workflow.js": true, ".claude/workflows/route_team.workflow.js": true,
		".git/hooks/pre-commit": true, ".git/hooks/commit-msg": true,
	}
	addSkill := func(name string) {
		skillDir := filepath.Join(".claude", "skills", name)
		allowed[filepath.ToSlash(filepath.Join(skillDir, "SKILL.md"))] = true
		allowed[filepath.ToSlash(filepath.Join(".claude", "skills", name+".md"))] = true
		allowed[filepath.ToSlash(filepath.Join(".claude", "skills", "autopus", name+".md"))] = true
		// Reference bodies are manifest-owned siblings of the SKILL.md, so
		// Clean must recognize them or it refuses the whole manifest.
		for _, rel := range pkgcontent.SkillResourceRelPaths(name) {
			allowed[filepath.ToSlash(filepath.Join(skillDir, filepath.FromSlash(rel)))] = true
		}
	}
	addSkill("auto")
	allowed[".claude/commands/auto.md"] = true
	allowed[".claude/commands/auto-workflows.md"] = true
	for _, route := range claudeWorkflowRoutes {
		name := "auto-" + route.name
		addSkill(name)
		allowed[filepath.ToSlash(filepath.Join(".claude", "commands", name+".md"))] = true
	}
	for _, dir := range []string{"skills", "agents", "rules", "hooks"} {
		entries, err := contentfs.FS.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read Claude managed %s names: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			switch dir {
			case "skills":
				if strings.HasSuffix(name, ".md") {
					addSkill(strings.TrimSuffix(name, ".md"))
				}
			case "agents":
				if strings.HasSuffix(name, ".md") {
					allowed[filepath.ToSlash(filepath.Join(".claude", "agents", "autopus", name))] = true
				}
			case "rules":
				if strings.HasSuffix(name, ".md") {
					allowed[filepath.ToSlash(filepath.Join(".claude", "rules", "autopus", name))] = true
				}
			case "hooks":
				allowed[filepath.ToSlash(filepath.Join(".claude", "hooks", "autopus", name))] = true
				if isClaudeRootHookFile(name) {
					allowed[filepath.ToSlash(filepath.Join(".claude", "hooks", name))] = true
				}
			}
		}
	}
	conditional, err := claudeConditionalRules()
	if err != nil {
		return nil, err
	}
	for _, mapping := range conditional.mappings {
		allowed[filepath.ToSlash(mapping.TargetPath)] = true
	}
	allowed[".claude/hooks/autopus/conditional-rules.json"] = true
	return allowed, nil
}
