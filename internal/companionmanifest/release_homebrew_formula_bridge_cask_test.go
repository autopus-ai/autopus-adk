package companionmanifest

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var frozenFormulaDigests = []string{
	"babce99376a647e801ea06d99f3575c87414551cbbeb77dfeed5cfa23851b964",
	"fbe9693d3517bdbaf92f230d7aa7561b728ba002749c2d06b6eef08170fed60b",
	"f150e713e2791116a2bc92e9893e202e5161c2f58fad3be55dfc08ba39f04b75",
	"8f331702c5d98418b45203d0b7b604f52a36d9e08b2a7dcbb6d5f6fe712ef878",
}

func TestHomebrewFormulaBridge_A32PinsCaskOnlyTapTransition(t *testing.T) {
	source := readReleaseFile(t, "scripts/companion-release/publish-homebrew-formula-bridge.sh")
	gitHelper := readReleaseFile(t,
		"scripts/companion-release/publish-homebrew-formula-bridge-git.sh")
	for _, required := range []string{
		"readonly RELEASE_TAG='v0.50.121'",
		"readonly RELEASE_VERSION='0.50.121'",
		"readonly PRIOR_TAP_COMMIT='ed38b4b2fd94e641c32f8ad637dd6f5f0cb50a73'",
		"readonly PRIOR_CASK_BLOB='5b1beab984c3fea1f6ff2da000825882cb9671e4'",
		"readonly FROZEN_FORMULA_BLOB='" + frozenFormulaBlob + "'",
		"readonly FORMULA_PATH='Formula/auto.rb'",
		"COMPANION_HOMEBREW_POLICY", "cask-only",
		`[[ -f "$git_helper" && ! -L "$git_helper" ]]`,
		`source "$git_helper"`,
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("A32 Homebrew caller policy missing %q", required)
		}
	}
	for _, required := range []string{
		"verify_frozen_formula", "verify_idempotent_head_snapshot",
		`api_json GET "git/commits/${head_sha}"`,
		`api_json GET "git/trees/${tree_sha}?recursive=1"`,
		"api_json POST 'git/blobs'", "api_json POST 'git/trees'",
		"api_json POST 'git/commits'", "api_json PATCH \"git/refs/heads/${TAP_BRANCH}\"",
		"'{base_tree:$base,tree:[{path:$path,mode:\"100644\",type:\"blob\",sha:$sha}]}'",
		"'{message:$message,tree:$tree,parents:[$parent]}'",
		"'{sha:$sha,force:false}'",
	} {
		if !strings.Contains(gitHelper, required) {
			t.Fatalf("A32 Homebrew Git CAS policy missing %q", required)
		}
	}
	implementation := source + "\n" + gitHelper
	for _, forbidden := range []string{
		"reconcile_tap_file formula Formula", "Publish signed Formula",
		"--method PUT",
	} {
		if strings.Contains(implementation, forbidden) {
			t.Fatalf("A32 production path can mutate the frozen Formula via %q", forbidden)
		}
	}
}

func TestHomebrewFormulaBridge_PublishedV05070CaskGolden(t *testing.T) {
	// This is the actual Casks/auto.rb published by the v2.17.0 release path.
	sum := sha256.Sum256([]byte(publishedV05070Cask))
	if got := fmt.Sprintf("%x", sum); got != "57d790fb79f8156aa83d5330be98c50b03d85b8a1175396b71bd642c3facc4b2" {
		t.Fatalf("published v0.50.70 Cask golden digest = %s", got)
	}
}

func TestHomebrewFormulaBridge_PublishedPredecessorTapPins(t *testing.T) {
	cask := homebrewBridgeCask()
	for _, required := range []string{
		`version "0.50.120"`,
		"b5a78f7afb4b9ea7e3439e02f7d0dad66dd0d8af2e4e90c9510b7c341ecb1d34",
		"2359b595f4fb3857b2e8c0b9ed849df3445586b7c6bd680367d610e2c71a0cdc",
		"fd2551898babf2c00fe5ec84146876e7750074eb65e878521fb6c770f30cf1e5",
		"3120e5acd77e996df534b85d6502139d60e4a3070d482e85044f9e2071b1e6db",
	} {
		if !strings.Contains(cask, required) {
			t.Fatalf("published v0.50.120 Cask missing %q", required)
		}
	}
	command := exec.Command("git", "hash-object", "--stdin")
	command.Stdin = strings.NewReader(cask)
	if blob, err := command.CombinedOutput(); err != nil || strings.TrimSpace(string(blob)) != "5b1beab984c3fea1f6ff2da000825882cb9671e4" {
		t.Fatalf("published v0.50.120 Cask blob = %q: %v", strings.TrimSpace(string(blob)), err)
	}
	formulaSum := sha256.Sum256([]byte(homebrewBridgeFormula(t)))
	if got := fmt.Sprintf("%x", formulaSum); got != "6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c" {
		t.Fatalf("frozen v0.50.71 Formula digest = %s", got)
	}
}

func TestHomebrewFormulaBridge_RejectsExecutableCaskStanzas(t *testing.T) {
	for _, stanza := range []string{"preflight", "postflight"} {
		t.Run(stanza, func(t *testing.T) {
			fixture := newHomebrewBridgeFixture(t)
			needle := "  binary \"auto\"\n"
			injection := fmt.Sprintf("%s\n  %s do\n    system \"/usr/bin/true\"\n  end\n", needle, stanza)
			malicious := strings.Replace(fixture.cask, needle, injection, 1)
			fixture.writeAPIContent(t, "cask.json", strings.Repeat("c", 40), malicious)

			output, err := fixture.run(nil)
			if err == nil || !strings.Contains(string(output), "published Cask differs from canonical v0.50.121") {
				t.Fatalf("%s Cask result: %v\n%s", stanza, err, output)
			}
			if got := fixture.updateCount(t, "cask"); got != "0" {
				t.Fatalf("%s Cask performed %s Cask updates", stanza, got)
			}
		})
	}
}

func homebrewBridgeCask() string {
	return strings.NewReplacer(
		`version "0.50.70"`, `version "0.50.120"`,
		"9728aec2f36bb43b4fbb658ca8550527d371a4c570ee7fbd2aee2b6fe011e8bd", "b5a78f7afb4b9ea7e3439e02f7d0dad66dd0d8af2e4e90c9510b7c341ecb1d34",
		"a57c0c180c0d2bb8ef013b9ae706752c432ff43466e13314b8b6f9279761fe4c", "2359b595f4fb3857b2e8c0b9ed849df3445586b7c6bd680367d610e2c71a0cdc",
		"f6ff6aba2ce96831b33570c07c2ec33353c8ee1cbfe9a53a2c62227f82bcf69b", "fd2551898babf2c00fe5ec84146876e7750074eb65e878521fb6c770f30cf1e5",
		"027f26f0bc2d3f052b28bbc2da80b15063f42f818be30bea132a78a601fc1822", "3120e5acd77e996df534b85d6502139d60e4a3070d482e85044f9e2071b1e6db",
	).Replace(publishedV05070Cask)
}

func homebrewBridgeFormula(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "formula.rb")
	renderer := filepath.Join(repositoryRootForBridge(),
		"scripts/companion-release/publish-homebrew-formula-bridge-render.sh")
	command := exec.Command("bash", "-c", `source "$1"
render_homebrew_formula_bridge "$2" v0.50.71 0.50.71 "$3" "$4" "$5" "$6"`,
		"render-formula", renderer, output, frozenFormulaDigests[0], frozenFormulaDigests[1],
		frozenFormulaDigests[2], frozenFormulaDigests[3])
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("render frozen Formula: %v\n%s", err, combined)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

const publishedV05070Cask = `# This file was generated by GoReleaser. DO NOT EDIT.
cask "auto" do
  version "0.50.70"

  on_macos do
    on_intel do
      sha256 "9728aec2f36bb43b4fbb658ca8550527d371a4c570ee7fbd2aee2b6fe011e8bd"
      url "https://github.com/Insajin/autopus-adk/releases/download/v#{version}/autopus-adk_#{version}_darwin_amd64.tar.gz"
    end
    on_arm do
      sha256 "a57c0c180c0d2bb8ef013b9ae706752c432ff43466e13314b8b6f9279761fe4c"
      url "https://github.com/Insajin/autopus-adk/releases/download/v#{version}/autopus-adk_#{version}_darwin_arm64.tar.gz"
    end
  end

  on_linux do
    on_intel do
      sha256 "f6ff6aba2ce96831b33570c07c2ec33353c8ee1cbfe9a53a2c62227f82bcf69b"
      url "https://github.com/Insajin/autopus-adk/releases/download/v#{version}/autopus-adk_#{version}_linux_amd64.tar.gz"
    end
    on_arm do
      sha256 "027f26f0bc2d3f052b28bbc2da80b15063f42f818be30bea132a78a601fc1822"
      url "https://github.com/Insajin/autopus-adk/releases/download/v#{version}/autopus-adk_#{version}_linux_arm64.tar.gz"
    end
  end

  name "auto"
  desc "Agentic Development Kit for coding CLIs (the auto CLI)"
  homepage "https://github.com/Insajin/autopus-adk"

  livecheck do
    skip "Auto-generated on release."
  end

  binary "auto"

  # No zap stanza required

end
`
