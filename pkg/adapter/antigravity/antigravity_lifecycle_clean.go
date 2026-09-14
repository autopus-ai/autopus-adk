package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// Clean removes only manifest-owned artifacts; shared trees are never deleted.
func (a *Adapter) Clean(_ context.Context) error {
	manifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return err
	}
	if manifest == nil {
		return a.removeRootMarker()
	}
	if _, owned := manifest.Files[".gemini/settings.json"]; owned {
		if err := a.removeManagedSettingsKeys(); err != nil {
			return err
		}
	}
	if _, owned := manifest.Files[antigravityHooksTarget]; owned {
		if err := a.removeAntigravityHooksJSON(); err != nil {
			return err
		}
	}
	roots := append(PruneRoots(), ".gemini/skills/auto", ".gemini/hooks/autopus", ".gemini/statusline.sh")
	entries := make([]adapter.ManifestDiffEntry, 0, len(manifest.Files))
	for path, recorded := range manifest.Files {
		clean := filepath.ToSlash(filepath.Clean(path))
		for _, root := range roots {
			if clean == root || strings.HasPrefix(clean, root+"/") {
				entries = append(entries, adapter.ManifestDiffEntry{Path: path, Action: adapter.ManifestActionPrune, OldChecksum: recorded.Checksum})
				break
			}
		}
	}
	// The shared primitive rejects escaping paths and durably backs up edits.
	var backupDir string
	if err := adapter.PruneManagedPaths(a.root, entries, &backupDir); err != nil {
		return err
	}
	return a.removeRootMarker()
}

func (a *Adapter) removeRootMarker() error {
	path := filepath.Join(a.root, "GEMINI.md")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(removeMarkerSection(string(raw))), 0o644)
}

// Mixed settings lose only matching authored values, never a user's event or
// server merely because it shares a name with an Autopus default.
func (a *Adapter) removeManagedSettingsKeys() error {
	path := filepath.Join(a.root, ".gemini", "settings.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("gemini settings cleanup: %w", err)
	}
	cfg, err := config.LoadPreview(a.root)
	if err != nil {
		return err
	}
	authored, err := a.renderAuthoredSettings(cfg)
	if err != nil {
		return err
	}
	removeAuthoredLegacyHooks(settings, append(defaultLegacyHookConfigs(), a.configuredLegacyGeminiHooks(cfg)...))
	currentServers, _ := settings["mcpServers"].(map[string]any)
	authoredServers, _ := authored["mcpServers"].(map[string]any)
	for name, value := range authoredServers {
		if reflect.DeepEqual(currentServers[name], value) {
			delete(currentServers, name)
		}
	}
	if len(currentServers) == 0 {
		delete(settings, "mcpServers")
	}
	if len(settings) == 0 {
		return os.Remove(path)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return adapter.WriteFileIfChanged(path, append(out, '\n'), 0o644)
}

func defaultLegacyHookConfigs() []adapter.HookConfig {
	hooks, _, _ := pkgcontent.GenerateProjectHookConfigs(config.DefaultFullConfig("autopus"), "gemini", true)
	return hooks
}

// Handler-level subtraction preserves user handlers sharing the same event
// and matcher. Unknown group shapes and customized handlers remain untouched.
func removeAuthoredLegacyHooks(settings map[string]any, authored []adapter.HookConfig) {
	events, ok := settings["hooks"].(map[string]any)
	if !ok {
		return
	}
	known := make(map[string]bool, len(authored))
	for _, hook := range authored {
		body, _ := json.Marshal(map[string]any{"type": hook.Type, "command": hook.Command, "timeout": hook.Timeout})
		known[hook.Event+"\x00"+hook.Matcher+"\x00"+string(body)] = true
	}
	for event, value := range events {
		groups, ok := value.([]any)
		if !ok {
			continue
		}
		keptGroups := make([]any, 0, len(groups))
		for _, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			handlers, ok := group["hooks"].([]any)
			if !ok {
				keptGroups = append(keptGroups, rawGroup)
				continue
			}
			matcher, _ := group["matcher"].(string)
			kept := make([]any, 0, len(handlers))
			for _, handler := range handlers {
				body, err := json.Marshal(handler)
				if err != nil || !known[event+"\x00"+matcher+"\x00"+string(body)] {
					kept = append(kept, handler)
				}
			}
			if len(kept) == len(handlers) {
				keptGroups = append(keptGroups, rawGroup)
			} else if len(kept) > 0 {
				group["hooks"] = kept
				keptGroups = append(keptGroups, group)
			}
		}
		if len(keptGroups) == 0 {
			delete(events, event)
		} else {
			events[event] = keptGroups
		}
	}
	if len(events) == 0 {
		delete(settings, "hooks")
	}
}
