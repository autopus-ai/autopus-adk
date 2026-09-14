package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

// PruneRoots returns every legacy or current Codex subtree eligible for manifest-owned update pruning.
func PruneRoots() []string {
	return []string{
		filepath.ToSlash(filepath.Join(".codex", "skills")),
		filepath.ToSlash(filepath.Join(".codex", "prompts")),
		filepath.ToSlash(filepath.Join(".codex", "rules")),
		filepath.ToSlash(filepath.Join(".agents", "skills")),
		filepath.ToSlash(filepath.Join(".autopus", "plugins", "auto", "skills")),
	}
}

var codexEmptyPrunePaths = []string{
	filepath.Join(".codex", "skills"),
	filepath.Join(".codex", "agents"),
	filepath.Join(".codex", "hooks", "autopus"),
	filepath.Join(".codex", "prompts"),
	filepath.Join(".codex", "rules", "autopus"),
	filepath.Join(".autopus", "plugins", "auto"),
}

func (a *Adapter) validateEmptyPruneRoots() error {
	for _, path := range codexEmptyPrunePaths {
		if err := adapter.RejectSymlinkComponents(a.root, path); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) pruneEmptyManagedDirs() error {
	if err := a.validateEmptyPruneRoots(); err != nil {
		return err
	}
	for _, path := range codexEmptyPrunePaths {
		if err := removeEmptyCodexDir(filepath.Join(a.root, path)); err != nil {
			return err
		}
	}
	return nil
}

// @AX:WARN [AUTO]: recursive empty-directory cleanup contains eight conditional branches.
// @AX:REASON [AUTO]: missing paths, symlink refusal, directory identity, recursive children, concurrent disappearance, and non-empty preservation converge here.
func removeEmptyCodexDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			if err := removeEmptyCodexDir(filepath.Join(path, entry.Name())); err != nil {
				return err
			}
		}
	}
	entries, err = os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || len(entries) != 0 {
		return err
	}
	return os.Remove(path)
}

func removeUnmodifiedCodexManagedFile(
	root, path string,
	owned adapter.ManifestFile,
) error {
	target, err := safeCodexCleanTarget(root, path)
	if err != nil || target == "" {
		return err
	}
	data, err := os.ReadFile(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if adapter.Checksum(string(data)) != owned.Checksum {
		return nil
	}
	return os.Remove(target)
}

func cleanCodexManagedMergeFile(root, path string, cleaner func(string) error) error {
	target, err := safeCodexCleanTarget(root, path)
	if err != nil || target == "" {
		return err
	}
	return cleaner(target)
}

func (a *Adapter) cleanRootDocMarkerSafely() error {
	target, err := safeCodexCleanTarget(a.root, "AGENTS.md")
	if err != nil || target == "" {
		return err
	}
	return a.cleanRootDocMarker()
}

func removeCodexManifestSafely(root string) error {
	path := filepath.Join(".autopus", adapterName+"-manifest.json")
	target, err := safeCodexCleanTarget(root, path)
	if err != nil || target == "" {
		return err
	}
	return os.Remove(target)
}

func safeCodexCleanTarget(root, path string) (string, error) {
	if err := adapter.RejectSymlinkComponents(root, path); err != nil {
		return "", err
	}
	return adapter.SafePruneFilePath(root, path)
}

func codexCleanAllowedPaths() (map[string]bool, error) {
	allowed := map[string]bool{
		".git/hooks/pre-commit": true, ".git/hooks/commit-msg": true,
		".autopus/plugins/auto/.codex-plugin/plugin.json": true,
	}
	addSkill := func(name string) {
		skillDirs := []string{
			filepath.Dir(codexProjectSkillPath(name)),
			filepath.Join(".agents", "skills", name),
			filepath.Join(".autopus", "plugins", "auto", "skills", name),
		}
		allowed[filepath.ToSlash(filepath.Join(".codex", "skills", name+".md"))] = true
		resources := pkgcontent.SkillResourceRelPaths(name)
		for _, dir := range skillDirs {
			allowed[filepath.ToSlash(filepath.Join(dir, "SKILL.md"))] = true
			// Reference bodies are manifest-owned siblings of the SKILL.md, so
			// Clean must recognize them or it refuses the whole manifest.
			for _, rel := range resources {
				allowed[filepath.ToSlash(filepath.Join(dir, filepath.FromSlash(rel)))] = true
			}
		}
	}
	for _, spec := range workflowSpecs {
		addSkill(spec.Name)
		allowed[filepath.ToSlash(filepath.Join(".codex", "prompts", spec.Name+".md"))] = true
	}
	skills, err := contentfs.FS.ReadDir("skills")
	if err != nil {
		return nil, fmt.Errorf("read managed skill names: %w", err)
	}
	for _, entry := range skills {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			addSkill(strings.TrimSuffix(entry.Name(), ".md"))
		}
	}
	agents, err := templates.FS.ReadDir(agentsTemplateDir)
	if err != nil {
		return nil, fmt.Errorf("read managed agent names: %w", err)
	}
	for _, entry := range agents {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmpl") {
			name := strings.TrimSuffix(entry.Name(), ".tmpl")
			allowed[filepath.ToSlash(filepath.Join(".codex", "agents", name))] = true
		}
	}
	for _, name := range codexHookAssetNames {
		allowed[filepath.ToSlash(filepath.Join(".codex", "hooks", "autopus", name))] = true
	}
	rules, err := contentfs.FS.ReadDir("rules")
	if err != nil {
		return nil, fmt.Errorf("read managed rule names: %w", err)
	}
	for _, entry := range rules {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := entry.Name()
		allowed[filepath.ToSlash(filepath.Join(".codex", "rules", "autopus", name))] = true
		allowed[filepath.ToSlash(filepath.Join(".codex", "rules", "autopus-"+name))] = true
		allowed[".codex/rules-autopus-"+name] = true
	}
	return allowed, nil
}
