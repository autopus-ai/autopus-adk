package companionmanifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v0.50.121 failed notarization twice with nothing but "submission failed":
// notarytool's stderr was discarded, so an unaccepted developer agreement was
// indistinguishable from an outage. The reason must reach the release log.
func TestDarwinReleaseProducer_NotaryFailureReportsAppleReason(t *testing.T) {
	_, artifact, output, err := runDarwinReleaseProducer(t, "notary_agreement_missing", false)
	if err == nil {
		t.Fatalf("producer accepted a failed notarization\n%s", output)
	}
	text := string(output)
	if !strings.Contains(text, "notarytool submission failed") {
		t.Fatalf("producer lost the stable failure message: %s", text)
	}
	if !strings.Contains(text, "notarytool: Error: HTTP status code: 403. A required agreement is missing or has expired.") {
		t.Fatalf("producer suppressed the notary rejection reason: %s", text)
	}
	if strings.Contains(text, "private-release-material") {
		t.Fatal("notary diagnostic leaked signing material")
	}
	assertNoDarwinReleaseMetadata(t, filepath.Dir(artifact))
}

func TestDarwinReleaseProducer_InvalidNotarizationReportsResponse(t *testing.T) {
	_, _, output, err := runDarwinReleaseProducer(t, "rejected_notarization", false)
	if err == nil {
		t.Fatalf("producer accepted an Invalid notarization\n%s", output)
	}
	if !strings.Contains(string(output), `notarytool response: {"status":"Invalid"`) {
		t.Fatalf("producer suppressed the Invalid notary response: %s", output)
	}
}

func fakeNotarytool(t *testing.T) {
	t.Helper()
	appendDarwinReleaseEvent(t, "accepted_notarization")
	switch os.Getenv("FAKE_DARWIN_SCENARIO") {
	case "notary_agreement_missing":
		fmt.Fprintln(os.Stderr, "Error: HTTP status code: 403. A required agreement is missing or has expired.")
		os.Exit(1)
	case "rejected_notarization":
		fmt.Printf(`{"status":"Invalid","id":"%s"}`, acceptedNotaryID)
	case "missing_notarization":
		fmt.Print(`{"status":"Accepted"}`)
	default:
		fmt.Printf(`{"status":"Accepted","id":"%s"}`, acceptedNotaryID)
	}
}
