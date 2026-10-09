package companionmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDarwinReleaseProducer_TrustGatesPrecedeManifestAndReceipt(t *testing.T) {
	dir, artifact, output, err := runDarwinReleaseProducer(t, "", false)
	if err != nil {
		t.Fatalf("producer failed: %v\n%s", err, output)
	}
	events, err := os.ReadFile(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"developer_id_sign", "notary_container", "accepted_notarization",
		"identity_verification", "notarization_ticket", "execution_smoke", "manifest_signature",
	}
	if got := strings.Fields(string(events)); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("release events = %v, want %v", got, wantEvents)
	}
	assertDarwinReceiptBindsOutputs(t, filepath.Dir(artifact), artifact)
}

func TestDarwinReleaseProducer_InvalidTrustEvidenceFailsClosed(t *testing.T) {
	cases := []string{
		"unsigned", "ad_hoc", "wrong_identifier", "wrong_team",
		"missing_runtime", "missing_timestamp", "rejected_notarization",
		"missing_notarization",
	}
	for _, scenario := range cases {
		t.Run(scenario, func(t *testing.T) {
			_, artifact, output, err := runDarwinReleaseProducer(t, scenario, false)
			if err == nil {
				t.Fatalf("producer accepted %s evidence\n%s", scenario, output)
			}
			assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
			if strings.Contains(string(output), "private-release-material") {
				t.Fatal("failure output leaked signing material")
			}
		})
	}
}

func TestDarwinReleaseProducer_PostManifestMutationFailsClosed(t *testing.T) {
	_, artifact, output, err := runDarwinReleaseProducer(t, "", true)
	if err == nil {
		t.Fatalf("producer accepted post-manifest mutation\n%s", output)
	}
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}

func TestDarwinReleaseProducer_SigningFailureReportsBoundedDiagnostic(t *testing.T) {
	_, artifact, output, err := runDarwinReleaseProducer(t, "signing_failure", false)
	if err == nil {
		t.Fatalf("producer accepted failed Developer ID signing\n%s", output)
	}
	if !strings.Contains(string(output), "codesign: fixture signing diagnostic 1") {
		t.Fatalf("producer suppressed codesign diagnostic: %s", output)
	}
	if strings.Contains(string(output), "codesign: fixture signing diagnostic 9") {
		t.Fatalf("producer emitted an unbounded codesign diagnostic: %s", output)
	}
	if strings.Contains(string(output), "private-release-material") {
		t.Fatal("signing diagnostic leaked private key")
	}
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}

func TestDarwinReleaseProducer_IdentityFailureReportsBoundedDiagnostic(t *testing.T) {
	_, artifact, output, err := runDarwinReleaseProducer(t, "identity_failure", false)
	if err == nil {
		t.Fatalf("producer accepted failed code identity verification\n%s", output)
	}
	if !strings.Contains(string(output), "codesign verify: fixture verification diagnostic 1") {
		t.Fatalf("producer suppressed code identity diagnostic: %s", output)
	}
	if strings.Contains(string(output), "codesign verify: fixture verification diagnostic 9") {
		t.Fatalf("producer emitted an unbounded code identity diagnostic: %s", output)
	}
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}

func TestDarwinReleaseProducer_RequirementCompilesWithNativeParser(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native code requirement parser is only available on macOS")
	}
	const marker = "designated_requirement='"
	requirement := string(readRepositoryFile(t, filepath.Join("scripts", "companion-release", "produce.sh")))
	start := strings.Index(requirement, marker)
	if start < 0 {
		t.Fatal("producer is missing the designated requirement")
	}
	requirement = requirement[start+len(marker):]
	end := strings.IndexByte(requirement, '\'')
	if end < 0 {
		t.Fatal("producer designated requirement is unterminated")
	}
	if output, err := exec.Command("/usr/bin/csreq", "-r="+requirement[:end], "-t").CombinedOutput(); err != nil {
		t.Fatalf("native code requirement parser rejected production policy: %v\n%s", err, output)
	}
}

func runDarwinReleaseProducer(t *testing.T, scenario string, mutate bool) (string, string, []byte, error) {
	t.Helper()
	dir := t.TempDir()
	artifactDir := filepath.Join(dir, "auto_darwin_arm64")
	if err := os.Mkdir(artifactDir, 0o700); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(artifactDir, "auto")
	if err := os.WriteFile(artifact, []byte("unsigned-auto"), 0o700); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "release-key")
	if err := os.WriteFile(keyFile, []byte("private-release-material"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := companionProducerEnv(
		artifact, "arm64", keyFile, writeSignerWrapper(t, dir),
		filepath.Join(dir, "signer-args"), filepath.Join(dir, "stdin-digest"),
	)
	environment = append(environment, append(darwinReleaseToolEnv(t, dir), releaseCanaryGateEnv(t, dir, "arm64")...)...)
	writeDarwinReleaseScenario(t, dir, scenario)
	environment = append(environment, "FAKE_DARWIN_SCENARIO="+scenario)
	if mutate {
		environment = append(environment, "FAKE_POST_MANIFEST_MUTATION=1")
	}
	command := exec.Command("bash", releaseProducerPath(t))
	command.Env = environment
	output, err := command.CombinedOutput()
	return dir, artifact, output, err
}

func TestDarwinReleaseToolHelper(t *testing.T) {
	if os.Getenv("GO_WANT_COMPANION_SIGNER_HELPER") != "1" {
		return
	}
	args := helperArguments(os.Args)
	if len(args) == 0 {
		t.Fatal("missing fake tool name")
	}
	switch args[0] {
	case "codesign":
		fakeCodesign(t, args[1:])
	case "ditto":
		appendDarwinReleaseEvent(t, "notary_container")
		copyFakeContainer(t, args[1:])
	case "xcrun":
		fakeNotarytool(t)
	case "plutil":
		fakePlutil(t, args[1:])
	case "shasum":
		fakeShasum(t, args[1:])
	default:
		t.Fatalf("unknown fake tool %q", args[0])
	}
	os.Exit(0)
}

func fakeCodesign(t *testing.T, args []string) {
	t.Helper()
	scenario := os.Getenv("FAKE_DARWIN_SCENARIO")
	artifact := args[len(args)-1]
	if containsArgument(args, "--sign") {
		for _, required := range []string{"--options", "runtime", "--timestamp", "--identifier", "co.autopus.adk"} {
			if !containsArgument(args, required) {
				t.Fatalf("codesign signing arguments missing %q: %v", required, args)
			}
		}
		if scenario == "signing_failure" {
			for index := 1; index <= 9; index++ {
				fmt.Fprintf(os.Stderr, "fixture signing diagnostic %d\n", index)
			}
			os.Exit(1)
		}
		appendDarwinReleaseEvent(t, "developer_id_sign")
		if scenario != "unsigned" {
			data, err := os.ReadFile(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(artifact, append(data, []byte("-developer-id")...), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if containsArgument(args, "--verify") {
		if !containsArgument(args, "--check-notarization") {
			t.Fatalf("codesign verification arguments missing %q: %v", "--check-notarization", args)
		}
		// The producer checks the offline identity requirement once, then the
		// full requirement with its notarized clause until the ticket resolves.
		switch {
		case containsArgument(args, "-R="+darwinSigningRequirement):
			appendDarwinReleaseEvent(t, "identity_verification")
		case containsArgument(args, "-R="+darwinSigningRequirement+" and notarized"):
			if scenario == "ticket_lag" && !darwinReleaseEventSeen(t, "notarization_ticket_pending") {
				appendDarwinReleaseEvent(t, "notarization_ticket_pending")
				os.Exit(1)
			}
			appendDarwinReleaseEvent(t, "notarization_ticket")
			return
		default:
			t.Fatalf("codesign verification requirement is not the release requirement: %v", args)
		}
		if scenario == "identity_failure" {
			for index := 1; index <= 9; index++ {
				fmt.Fprintf(os.Stderr, "fixture verification diagnostic %d\n", index)
			}
			os.Exit(1)
		}
		if scenario == "unsigned" || scenario == "ad_hoc" {
			os.Exit(1)
		}
		return
	}
	identifier, team := "co.autopus.adk", "GP2PFA2PUV"
	if scenario == "wrong_identifier" {
		identifier = "co.invalid.adk"
	}
	if scenario == "wrong_team" {
		team = "AAAAAAAAAA"
	}
	lines := []string{"Executable=" + artifact, "Identifier=" + identifier, "TeamIdentifier=" + team}
	if scenario != "missing_timestamp" {
		lines = append(lines, "Timestamp=Jul 14, 2026 at 12:00:00")
	}
	flags := "0x10000(runtime)"
	if scenario == "missing_runtime" {
		flags = "0x0(none)"
	}
	lines = append(lines, "CodeDirectory v=20500 size=512 flags="+flags+" hashes=8+2 location=embedded")
	fmt.Fprintln(os.Stderr, strings.Join(lines, "\n"))
}

func fakePlutil(t *testing.T, args []string) {
	t.Helper()
	field, path := args[1], args[len(args)-1]
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{}
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	value, ok := values[field].(string)
	if !ok {
		os.Exit(1)
	}
	fmt.Print(value)
}

func fakeShasum(t *testing.T, args []string) {
	t.Helper()
	path := args[len(args)-1]
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	fmt.Printf("%s  %s\n", hex.EncodeToString(sum[:]), path)
}

func copyFakeContainer(t *testing.T, args []string) {
	t.Helper()
	data, err := os.ReadFile(args[len(args)-2])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(args[len(args)-1], data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
