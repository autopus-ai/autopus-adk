package record

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/scaffold"
)

// LoadPacks loads the project's Journey Packs and, in memory only, gives a
// playwright pack that declares no gui.allowed_origins the baseURL of the
// project's Playwright config. `auto qa init` emits exactly such a pack for
// browser-staging, and its specs run against that baseURL, so the baseURL is
// the origin the project already declared for it. Nothing is written back.
func LoadPacks(projectDir string) ([]journey.Pack, error) {
	packs, err := journey.LoadDir(projectDir)
	if err != nil {
		return packs, err
	}
	baseURL := ""
	for i := range packs {
		if packs[i].Adapter.ID != "playwright" || len(packs[i].GUI.AllowedOrigins) > 0 {
			continue
		}
		if baseURL == "" {
			baseURL = scaffold.DetectPlaywrightOrigin(projectDir)
		}
		if baseURL != "" {
			packs[i].GUI.AllowedOrigins = []string{baseURL}
		}
	}
	return packs, nil
}

// recordedOrigin is the origin of the first absolute goto in a recording.
// Codegen writes absolute URLs, so a recording names the origin it was made
// against even when no pack or flag does.
func recordedOrigin(rec Recording) string {
	for _, event := range rec.Events {
		if event.Action != "goto" || !strings.Contains(event.URL, "://") {
			continue
		}
		if origin, err := CanonicalOrigin(event.URL); err == nil {
			return origin
		}
	}
	return ""
}
