package companionmanifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Apple's ticket lookup can lag an Accepted notarytool verdict; the producer
// retries only the notarized clause and still publishes once it resolves.
func TestDarwinReleaseProducer_NotarizationTicketLagIsRetried(t *testing.T) {
	dir, artifact, output, err := runDarwinReleaseProducer(t, "ticket_lag", false)
	if err != nil {
		t.Fatalf("producer failed on a lagging ticket: %v\n%s", err, output)
	}
	events, err := os.ReadFile(filepath.Join(dir, "events"))
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"developer_id_sign", "notary_container", "accepted_notarization", "identity_verification",
		"notarization_ticket_pending", "notarization_ticket", "execution_smoke", "manifest_signature",
	}
	if got := strings.Fields(string(events)); !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("release events = %v, want %v", got, wantEvents)
	}
	assertDarwinReceiptBindsOutputs(t, filepath.Dir(artifact), artifact)
}

const darwinSigningRequirement = `identifier "co.autopus.adk" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "GP2PFA2PUV"`

func darwinReleaseEventSeen(t *testing.T, event string) bool {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("FAKE_DARWIN_EVENTS"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, seen := range strings.Fields(string(data)) {
		if seen == event {
			return true
		}
	}
	return false
}
