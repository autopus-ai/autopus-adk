package companionmanifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// GoReleaser renders the Cask that publish-homebrew-formula-bridge.sh compares
// byte for byte with the canonical renderer. Without an explicit URL template
// GoReleaser takes the owner from the release repository, so after the transfer
// to autopus-ai its URLs stopped matching the renderer's and every release
// failed at "Publish Homebrew Cask". The two must name the same download path.
func TestHomebrewCaskURLTemplate_MatchesCanonicalRenderer(t *testing.T) {
	var config struct {
		HomebrewCasks []struct {
			Name string `yaml:"name"`
			URL  struct {
				Template string `yaml:"template"`
			} `yaml:"url"`
		} `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal([]byte(readReleaseFile(t, ".goreleaser.yaml")), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.HomebrewCasks) != 1 || config.HomebrewCasks[0].Name != "auto" {
		t.Fatalf("expected exactly one auto Cask, got %+v", config.HomebrewCasks)
	}
	const prefix = "https://github.com/Insajin/autopus-adk/releases/download/"
	template := config.HomebrewCasks[0].URL.Template
	if template != prefix+"{{ urlPathEscape .Tag }}/{{ .ArtifactName }}" {
		t.Fatalf("Cask URL template = %q", template)
	}

	// GoReleaser replaces the version inside the rendered URL with #{version},
	// so tag v<version> and the archive name reduce to the renderer's form.
	renderer := readReleaseFile(t, "scripts/companion-release/publish-homebrew-formula-bridge-render.sh")
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		version := "0.50.69"
		rendered := strings.ReplaceAll(prefix+"v"+version+"/autopus-adk_"+version+"_"+target+".tar.gz", version, "#{version}")
		if !strings.Contains(renderer, `url "`+rendered+`"`) {
			t.Fatalf("canonical renderer has no URL %q", rendered)
		}
	}
}
