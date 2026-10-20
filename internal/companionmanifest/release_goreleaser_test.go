package companionmanifest

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

const productionGoReleaserModule = "github.com/goreleaser/goreleaser/v2@v2.17.0"

var productionGoReleaserFixtureRuns atomic.Int32

func TestProductionGoReleaserRunner_UsesExactPinnedVersion(t *testing.T) {
	command := exactGoReleaserCommand("--version")
	want := "go run " + productionGoReleaserModule + " --version"
	if got := strings.Join(command.Args, " "); got != want {
		t.Fatalf("production GoReleaser command = %q, want %q", got, want)
	}
}

func TestProductionGoReleaserFixture_RequiresIntegrationTag(t *testing.T) {
	if executableReleaseIntegrationEnabled {
		t.Skip("non-integration contract")
	}
	if runs := productionGoReleaserFixtureRuns.Load(); runs != 0 {
		t.Fatalf("non-integration GoReleaser fixture runs = %d, want 0", runs)
	}
}

func runProductionGoReleaser(t *testing.T, tools mockReleaseTools) map[string]string {
	t.Helper()
	requireExecutableReleaseIntegration(t)
	if err := validateProductionGoReleaserWiring(readReleaseFile(t, ".goreleaser.yaml")); err != nil {
		t.Fatalf("production GoReleaser wiring: %v", err)
	}
	archives, err := runGoReleaserFixture(t, tools, nil, []string{"amd64", "arm64"})
	if err != nil {
		t.Fatalf("production GoReleaser archive path failed: %v", err)
	}
	return archives
}

type goReleaserConfigMutation func(*testing.T, string) string

func runGoReleaserFixture(
	t *testing.T,
	tools mockReleaseTools,
	mutation goReleaserConfigMutation,
	architectures []string,
) (map[string]string, error) {
	t.Helper()
	requireExecutableReleaseIntegration(t)
	root := filepath.Join(t.TempDir(), "repository")
	copyGoReleaserRepository(t, root)
	if mutation != nil {
		configPath := filepath.Join(root, ".goreleaser.yaml")
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, []byte(mutation(t, string(config))), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runFixtureCommand(t, root, "git", "init", "-q")
	disableReleaseTestGitMaintenance(t, func(arguments ...string) {
		runFixtureCommand(t, root, "git", arguments...)
	})
	runFixtureCommand(t, root, "git", "config", "user.name", "F07 Release Test")
	runFixtureCommand(t, root, "git", "config", "user.email", "f07@example.invalid")
	runFixtureCommand(t, root, "git", "remote", "add", "origin", "https://example.invalid/Insajin/autopus-adk.git")
	runFixtureCommand(t, root, "git", "add", ".")
	runFixtureCommand(t, root, "git", "commit", "-qm", "F07 archive fixture")
	runFixtureCommand(t, root, "git", "tag", "v0.50.69")
	commit := strings.TrimSpace(runFixtureCommand(t, root, "git", "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runFixtureCommand(t, root, "git", "rev-parse", "HEAD^{tree}"))

	credentials := filepath.Join(filepath.Dir(root), "credentials")
	if err := os.Mkdir(credentials, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(credentials, "release-key")
	apiKeyPath := filepath.Join(credentials, "AuthKey_FIXTURE.p8")
	if err := os.WriteFile(keyPath, encodedReleaseKey(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apiKeyPath, []byte("fixture-api-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmpDir := filepath.Join(filepath.Dir(root), "tmp")
	if err := os.Mkdir(tmpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryRoot := filepath.Join(filepath.Dir(root), "omp-release-canary-root")
	if err := os.Mkdir(canaryRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	canaryExecutable := filepath.Join(canaryRoot, "omp-darwin-arm64")
	if err := os.WriteFile(canaryExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o555); err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, ".autopus", "runtime", "release-evidence")
	if err := os.MkdirAll(evidenceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	promotionReportPath := filepath.Join(".autopus", "runtime", "release-evidence",
		"omp-context-promotion-report.v1.json")
	promotionAttestationPath := filepath.Join(".autopus", "runtime", "release-evidence",
		"omp-context-promotion-attestation.v2.json")
	for _, path := range []string{promotionReportPath, promotionAttestationPath} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	command := exactGoReleaserCommand("release", "--clean", "--parallelism=2",
		"--skip=announce,publish,sign,homebrew")
	command.Dir = root
	command.Env = append(os.Environ(), goReleaserReleaseEnv(
		tools, keyPath, apiKeyPath, tmpDir, commit, tree, canaryRoot, canaryExecutable,
		promotionReportPath, promotionAttestationPath, fixtureOMPStaticPolicyB64(t, commit, tree),
	)...)
	productionGoReleaserFixtureRuns.Add(1)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("goreleaser: %w\n%s", err, output)
	}
	archives := make(map[string]string)
	for _, architecture := range architectures {
		name := fmt.Sprintf("autopus-adk_0.50.69_darwin_%s.tar.gz", architecture)
		path := filepath.Join(root, "dist", name)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("production GoReleaser did not create %s: %w\n%s", name, err, output)
		}
		archives[architecture] = path
	}
	return archives, nil
}

func requireExecutableReleaseIntegration(t *testing.T) {
	t.Helper()
	if !executableReleaseIntegrationEnabled {
		t.Skip("executable GoReleaser fixture requires -tags integration")
	}
}

func exactGoReleaserCommand(arguments ...string) *exec.Cmd {
	goArguments := append([]string{"run", productionGoReleaserModule}, arguments...)
	return exec.Command("go", goArguments...)
}

func goReleaserReleaseEnv(
	tools mockReleaseTools,
	keyPath, apiKeyPath, tmpDir, commit, tree, canaryRoot, canaryExecutable,
	promotionReportPath, promotionAttestationPath, staticPolicyB64 string,
) []string {
	return []string{
		"TMPDIR=" + tmpDir,
		"GITHUB_REF_NAME=v0.50.69",
		"COMPANION_SOURCE_COMMIT=" + commit,
		"COMPANION_SOURCE_TREE=" + tree,
		"OMP_CONTEXT_PROMOTION_REPORT_PATH=" + promotionReportPath,
		"OMP_CONTEXT_PROMOTION_ATTESTATION_PATH=" + promotionAttestationPath,
		"OMP_CONTEXT_STATIC_POLICY_B64=" + staticPolicyB64,
		"OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256=" + strings.Repeat("0", 64),
		"OMP_CONTEXT_RELEASE_CANARY_ROOT=" + canaryRoot,
		"OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE=" + canaryExecutable,
		"COMPANION_BUILD_PROVENANCE=github-actions:Insajin/autopus-adk@" + commit,
		"COMPANION_HANDOFF=v1", "COMPANION_ROLLBACK_FLOOR=5069",
		"COMPANION_ISSUED_AT=2026-07-15T00:00:00Z",
		"COMPANION_EXPIRES_AT=2026-07-16T00:00:00Z",
		"COMPANION_KEY_ID=release-key", "COMPANION_SIGNING_KEY_FILE=" + keyPath,
		"COMPANION_SIGNER=" + tools.signer, "COMPANION_RECEIPT_VERIFIER=" + tools.verifier,
		"COMPANION_EXEC_SMOKE_GATE=" + tools.tools["exec-smoke-gate"],
		"COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT=2026-07-14T00:00:00Z",
		"COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT=2027-07-15T00:00:00Z",
		"COMPANION_PUBLIC_KEY_RECEIPT_MINIMUM_LIFETIME_SECONDS=31536000",
		"APPLE_SIGNING_IDENTITY=Developer ID Application: Fixture (GP2PFA2PUV)",
		"APPLE_API_KEY=FIXTUREKEY",
		"APPLE_API_ISSUER=123e4567-e89b-42d3-a456-426614174000",
		"APPLE_API_KEY_PATH=" + apiKeyPath,
		"COMPANION_CODESIGN_TOOL=" + tools.tools["codesign"],
		"COMPANION_DITTO_TOOL=" + tools.tools["ditto"],
		"COMPANION_XCRUN_TOOL=" + tools.tools["xcrun"],
		"COMPANION_PLUTIL_TOOL=" + tools.tools["plutil"],
		"COMPANION_SHASUM_TOOL=" + tools.tools["shasum"],
		"HOMEBREW_TAP_TOKEN=fixture-token",
	}
}

func fixtureOMPStaticPolicyB64(t *testing.T, commit, tree string) string {
	t.Helper()
	hash := "sha256:" + strings.Repeat("a", 64)
	policy := promptlayer.OMPContextPromotionStaticPolicyV3{
		SchemaVersion:      promptlayer.OMPContextPromotionRuntimeSchemaV3,
		ProducerRepository: "Insajin/autopus-adk-evals", ProducerWorkflowRef: "release-fixture.yml@immutable",
		CandidateRepository: "Insajin/autopus-adk", SourceCommit: commit, SourceTree: tree,
		Target: "darwin-arm64", AutoVersion: "0.50.69", PolicyID: "fixture-policy", PolicyDigest: hash,
		OMPVersion: "v17.2.7", OMPExecutableSHA256: hash, PipelineImplementationDigest: hash,
		Provider: "openai", ProviderAuthorityDigest: hash, ModelScopeDigest: hash,
		CohortManifestDigest: hash, OrderSeed: hash, OraclePolicyDigest: hash,
		PromotionSigningKeyID: promptlayer.OMPContextPromotionKeyID2026Q3K3,
		ReleaseLineageKeyID:   "fixture-lineage-key", ReleaseLineageHandoff: "v1",
		MinimumRollbackFloor: 5069,
	}
	body, err := promptlayer.MarshalOMPContextPromotionStaticPolicyV3(policy)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

func copyGoReleaserRepository(t *testing.T, destination string) {
	t.Helper()
	root := repositoryRoot(t)
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd", "content", "internal", "pkg", "scripts", "templates"} {
		copyReleasePath(t, filepath.Join(root, name), filepath.Join(destination, name))
	}
	for _, name := range []string{
		".gitignore", ".goreleaser.yaml", "go.mod", "go.sum", "LICENSE", "README.md", "CHANGELOG.md",
	} {
		copyReleasePath(t, filepath.Join(root, name), filepath.Join(destination, name))
	}
}

func copyReleasePath(t *testing.T, source, destination string) {
	t.Helper()
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("release fixture refuses symlink %s", source)
	}
	if info.IsDir() {
		if err := os.Mkdir(destination, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			copyReleasePath(t, filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()))
		}
		return
	}
	copyReleaseFile(t, source, destination, info.Mode())
}

func copyReleaseFile(t *testing.T, source, destination string, mode fs.FileMode) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		_ = input.Close()
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = input.Close()
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		_ = output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func runFixtureCommand(t *testing.T, dir, name string, arguments ...string) string {
	t.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, arguments, err, output)
	}
	return string(output)
}
