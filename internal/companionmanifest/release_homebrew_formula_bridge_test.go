package companionmanifest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The tap coordinates advance with every publication. Reading them from the
// publisher keeps these fixtures from silently drifting out of the head check
// and testing nothing; the frozen Formula blob is a real constant, so it stays.
const frozenFormulaBlob = "4ebc6c38925002dec00759823d4dd847a499818a"

func tapPin(t *testing.T, name string) string {
	t.Helper()
	const bridge = "scripts/companion-release/publish-homebrew-formula-bridge.sh"
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(bridge)))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^readonly ` + name + `='([0-9a-f]{40})'$`).FindSubmatch(data)
	if match == nil {
		t.Fatalf("%s does not pin %s", bridge, name)
	}
	return string(match[1])
}

var bridgeDigests = []string{
	strings.Repeat("1", 64), strings.Repeat("2", 64),
	strings.Repeat("3", 64), strings.Repeat("4", 64),
}

func TestHomebrewFormulaBridge_A31UpdatesOnlyCaskThenIsIdempotent(t *testing.T) {
	fixture := newHomebrewBridgeFixture(t)
	frozenFormula, err := os.ReadFile(filepath.Join(fixture.state, "formula.json"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := fixture.run(nil)
	if err != nil {
		t.Fatalf("publish A31 Cask: %v\n%s", err, output)
	}
	if cask := fixture.apiContent(t, "cask.json"); !strings.Contains(cask, `version "0.50.120"`) {
		t.Fatalf("published Cask is not v0.50.120:\n%s", cask)
	}
	if got := fixture.updateCount(t, "cask"); got != "1" {
		t.Fatalf("Cask update count = %q, want 1", got)
	}
	if got := fixture.updateCount(t, "formula"); got != "0" {
		t.Fatalf("Formula update count = %q, want 0", got)
	}
	if got := fixture.callCount(t, "formula"); got != "1" {
		t.Fatalf("Formula snapshot read count = %q, want 1", got)
	}
	formulaAfter, err := os.ReadFile(filepath.Join(fixture.state, "formula.json"))
	if err != nil || string(formulaAfter) != string(frozenFormula) {
		t.Fatalf("frozen v0.50.71 Formula record changed: %v", err)
	}
	output, err = fixture.run(nil)
	if err != nil || !strings.Contains(string(output), "Cask is already current") {
		t.Fatalf("idempotent run: %v\n%s", err, output)
	}
	if got := fixture.updateCount(t, "cask"); got != "1" {
		t.Fatalf("idempotent Cask update count = %q, want 1", got)
	}
	if got := fixture.updateCount(t, "formula"); got != "0" {
		t.Fatalf("idempotent Formula update count = %q, want 0", got)
	}
	output, err = fixture.run(map[string]string{"HOMEBREW_TAP_TOKEN": "", "GH_TOKEN": "fallback-token"})
	if calls := fixture.callCount(t, "formula"); err != nil || calls != "3" {
		t.Fatalf("GH_TOKEN fallback: %v, Formula calls=%s\n%s", err, calls, output)
	}
}

func TestHomebrewFormulaBridge_RejectsCaskMismatch(t *testing.T) {
	t.Run("published Cask mismatch", func(t *testing.T) {
		fixture := newHomebrewBridgeFixture(t)
		mismatch := strings.Replace(fixture.cask, bridgeDigests[0], strings.Repeat("f", 64), 1)
		fixture.writeAPIContent(t, "cask.json", strings.Repeat("a", 40), mismatch)
		output, err := fixture.run(nil)
		if err == nil || !strings.Contains(string(output), "Cask") {
			t.Fatalf("Cask mismatch result: %v\n%s", err, output)
		}
		if got := fixture.updateCount(t, "cask"); got != "0" {
			t.Fatalf("Cask mismatch performed %s updates", got)
		}
	})
}

func TestHomebrewFormulaBridge_RejectsChecksumAmbiguity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"duplicate", func(value string) string { return value + strings.Split(value, "\n")[0] + "\n" }, "duplicate archive"},
		{"missing", func(value string) string { return strings.Join(strings.Split(value, "\n")[1:], "\n") }, "missing an exact bridge archive"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHomebrewBridgeFixture(t)
			if err := os.WriteFile(fixture.checksums, []byte(test.mutate(fixture.checksumText)), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := fixture.run(nil)
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("checksum %s result: %v\n%s", test.name, err, output)
			}
		})
	}
}

func TestHomebrewFormulaBridge_RejectsIdentityMismatchWithoutCredentialLeak(t *testing.T) {
	for _, test := range []struct{ name, key, value string }{
		{"tag", "GITHUB_REF_NAME", "v0.50.74"},
		{"version", "COMPANION_VERSION", "0.50.74"},
		{"policy", "COMPANION_HOMEBREW_POLICY", "formula-update"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHomebrewBridgeFixture(t)
			output, err := fixture.run(map[string]string{test.key: test.value})
			if err == nil || !strings.Contains(strings.ToLower(string(output)), "policy") {
				t.Fatalf("identity mismatch result: %v\n%s", err, output)
			}
		})
	}
	t.Run("credential", func(t *testing.T) {
		fixture := newHomebrewBridgeFixture(t)
		secret := "tap-token-must-never-appear"
		output, err := fixture.run(map[string]string{
			"HOMEBREW_TAP_TOKEN": secret, "MOCK_FAIL_WITH_TOKEN": "1",
		})
		if err == nil || strings.Contains(string(output), secret) {
			t.Fatalf("credential failure leaked token: %v\n%s", err, output)
		}
	})
}

type homebrewBridgeFixture struct {
	root, state, checksums, checksumText, cask string
	// priorCommit mirrors the publisher's tap-head pin so the relocated mock
	// fixture answers with the same coordinate the publisher enforces.
	priorCommit string
}

func newHomebrewBridgeFixture(t *testing.T) homebrewBridgeFixture {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	bin := filepath.Join(root, "bin")
	for _, path := range []string{state, bin, filepath.Join(root, "tmp")} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture := homebrewBridgeFixture{root: root, state: state,
		priorCommit: tapPin(t, "PRIOR_TAP_COMMIT")}
	fixture.cask = homebrewBridgeCask()
	fixture.checksumText = homebrewBridgeChecksums()
	fixture.checksums = filepath.Join(root, "checksums.txt")
	if err := os.WriteFile(fixture.checksums, []byte(fixture.checksumText), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.writeAPIContent(t, "cask.json", tapPin(t, "PRIOR_CASK_BLOB"), fixture.cask)
	fixture.writeAPIContent(t, "formula.json", frozenFormulaBlob, homebrewBridgeFormula(t))
	branch := `{"ref":"refs/heads/main","object":{"type":"commit","sha":"` +
		tapPin(t, "PRIOR_TAP_COMMIT") + `","url":"https://example.invalid/prior-commit"}}`
	if err := os.WriteFile(filepath.Join(state, "branch.json"), []byte(branch), 0o600); err != nil {
		t.Fatal(err)
	}
	mockPath := filepath.Join(repositoryRootForBridge(),
		"scripts/companion-release/tests/testdata/mock-tap-gh.sh")
	mock, err := os.ReadFile(mockPath)
	if err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "gh"), string(mock))
	return fixture
}

func (fixture homebrewBridgeFixture) run(overrides map[string]string) ([]byte, error) {
	environment := map[string]string{
		"PATH":   filepath.Join(fixture.root, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TMPDIR": filepath.Join(fixture.root, "tmp"), "MOCK_TAP_STATE": fixture.state,
		"GITHUB_REF_NAME": "v0.50.120", "COMPANION_VERSION": "0.50.120",
		"COMPANION_HOMEBREW_POLICY": "cask-only",
		"MOCK_TAP_PRIOR_COMMIT":     fixture.priorCommit,
		"COMPANION_CHECKSUMS_PATH":  fixture.checksums,
		"HOMEBREW_TAP_TOKEN":        "fixture-tap-token", "GH_TOKEN": "",
	}
	for name, value := range overrides {
		environment[name] = value
	}
	command := exec.Command("bash", filepath.Join(repositoryRootForBridge(),
		"scripts/companion-release/publish-homebrew-formula-bridge.sh"))
	command.Env = makeBridgeEnvironment(environment)
	return command.CombinedOutput()
}

func makeBridgeEnvironment(values map[string]string) []string {
	blocked := map[string]bool{}
	for name := range values {
		blocked[name] = true
	}
	environment := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if !blocked[name] {
			environment = append(environment, entry)
		}
	}
	for name, value := range values {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func repositoryRootForBridge() string {
	root, _ := filepath.Abs(filepath.Join("..", ".."))
	return root
}

func (fixture homebrewBridgeFixture) writeAPIContent(t *testing.T, name, sha, content string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"sha": sha, "content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.state, name), payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (fixture homebrewBridgeFixture) apiContent(t *testing.T, name string) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(fixture.state, name))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]string
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(response["content"])
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func (fixture homebrewBridgeFixture) updateCount(t *testing.T, name string) string {
	t.Helper()
	counter := name + ".updates"
	if name == "cask" {
		counter = "ref-update.calls"
	}
	value, err := os.ReadFile(filepath.Join(fixture.state, counter))
	if os.IsNotExist(err) {
		return "0"
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(value))
}

func (fixture homebrewBridgeFixture) callCount(t *testing.T, name string) string {
	t.Helper()
	counter := name + ".calls"
	if name == "formula" {
		counter = "formula-get.calls"
	}
	value, err := os.ReadFile(filepath.Join(fixture.state, counter))
	if os.IsNotExist(err) {
		return "0"
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(value))
}

func homebrewBridgeChecksums() string {
	names := []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}
	var output strings.Builder
	for index, name := range names {
		fmt.Fprintf(&output, "%s  autopus-adk_0.50.120_%s.tar.gz\n", bridgeDigests[index], name)
	}
	fmt.Fprintf(&output, "%s  autopus-adk_0.50.120_windows_amd64.zip\n", strings.Repeat("5", 64))
	return output.String()
}
