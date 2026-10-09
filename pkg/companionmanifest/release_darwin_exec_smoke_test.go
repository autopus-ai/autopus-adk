package companionmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeReleaseCanaryFixture(t *testing.T, dir string) {
	t.Helper()
	root := filepath.Join(dir, "omp-release-canary-root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"home", "tmp"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	canary := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$#\" -eq 1 && \"$1\" == '--version' ]]\nprintf executed >%q\nprintf 'omp/17.2.7\\n'\n",
		filepath.Join(dir, "omp-release-canary.executed"))
	if err := os.WriteFile(filepath.Join(root, "omp-darwin-arm64"), []byte(canary), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "omp-darwin-arm64"), 0o555); err != nil {
		t.Fatal(err)
	}
}

func releaseCanaryGateEnv(t *testing.T, dir, architecture string) []string {
	t.Helper()
	gate := filepath.Join(dir, "release-canary-gate")
	root := filepath.Join(dir, "omp-release-canary-root")
	canary := filepath.Join(root, "omp-darwin-arm64")
	canaryBody, err := os.ReadFile(canary)
	if err != nil {
		t.Fatal(err)
	}
	canaryDigest := sha256.Sum256(canaryBody)
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
[[ "$#" -eq 8 && "$1" == '--artifact' && -x "$2" && "$3" == '--expected-version' && -n "$4" &&
   "$5" == '--architecture' && "$6" == %q && "$7" == '--timeout' && "$8" == '45s' ]] || exit 91
scenario=''; [[ ! -f %q ]] || scenario="$(cat %q)"
[[ "$scenario" != 'execution_smoke_failure' ]] || exit 42
if [[ "$6" == 'arm64' ]]; then
  root="${OMP_CONTEXT_RELEASE_CANARY_ROOT-}"
  canary="${OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE-}"
  root_mode=$(stat -f '%%Lp' "$root" 2>/dev/null || true)
  [[ "$root_mode" =~ ^[0-7]{3,4}$ ]] || root_mode=$(stat -c '%%a' "$root")
  [[ "$root" == %q && -d "$root" && ! -L "$root" && "$root_mode" == '755' &&
     "$canary" == %q && -f "$canary" && ! -L "$canary" && -x "$canary" &&
     "$(shasum -a 256 "$canary" | awk '{print $1}')" == %q && "$("$canary" --version)" == 'omp/17.2.7' ]] || exit 92
else
  [[ -z "${OMP_CONTEXT_RELEASE_CANARY_ROOT-}" && -z "${OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE-}" ]] || exit 95
fi
[[ "$scenario" != 'execution_smoke_mutation' ]] || printf mutated >>"$2"
printf 'execution_smoke\n' >>%q
`, architecture, filepath.Join(dir, "scenario"), filepath.Join(dir, "scenario"),
		root, canary, hex.EncodeToString(canaryDigest[:]), filepath.Join(dir, "events"))
	if err := os.WriteFile(gate, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return []string{"COMPANION_EXEC_SMOKE_GATE=" + gate}
}

func assertReleaseCanaryExecuted(t *testing.T, dir string, want bool) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(dir, "omp-release-canary.executed"))
	if want && (err != nil || string(contents) != "executed") {
		t.Fatalf("arm64 release canary execution = %q, error = %v", contents, err)
	}
	if !want && !os.IsNotExist(err) {
		t.Fatalf("non-arm64 release unexpectedly executed OMP canary: %v", err)
	}
}

func TestDarwinReleaseProducer_ExecutionSmokeFailureFailsClosed(t *testing.T) {
	dir, artifact, output, err := runDarwinReleaseProducer(t, "execution_smoke_failure", false)
	if err == nil {
		t.Fatalf("producer accepted failed execution smoke\n%s", output)
	}
	if !strings.Contains(string(output), "final signed companion execution smoke failed") {
		t.Fatalf("producer did not report execution smoke failure: %s", output)
	}
	events, readErr := os.ReadFile(filepath.Join(dir, "events"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	wantEvents := []string{
		"developer_id_sign", "notary_container", "accepted_notarization",
		"identity_verification", "notarization_ticket",
	}
	if got := strings.Fields(string(events)); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("release events = %v, want %v", got, wantEvents)
	}
	assertReleaseCanaryExecuted(t, dir, false)
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}

func TestDarwinReleaseProducer_ExecutionSmokeMutationFailsClosed(t *testing.T) {
	dir, artifact, output, err := runDarwinReleaseProducer(t, "execution_smoke_mutation", false)
	if err == nil {
		t.Fatalf("producer accepted artifact mutation during execution smoke\n%s", output)
	}
	if !strings.Contains(string(output), "changed during execution smoke") {
		t.Fatalf("producer did not report execution-smoke mutation: %s", output)
	}
	events, readErr := os.ReadFile(filepath.Join(dir, "events"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	wantEvents := []string{
		"developer_id_sign", "notary_container", "accepted_notarization",
		"identity_verification", "notarization_ticket", "execution_smoke",
	}
	if got := strings.Fields(string(events)); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("release events = %v, want %v", got, wantEvents)
	}
	assertReleaseCanaryExecuted(t, dir, true)
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}
