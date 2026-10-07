package companionmanifest

import (
	"os"
	"strings"
	"testing"
)

func TestSignedPairLockPlatformContracts_AreNeverNoOp(t *testing.T) {
	windowsSource, err := os.ReadFile("../oslock/oslock_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"windows.LockFileEx",
		"windows.LOCKFILE_EXCLUSIVE_LOCK",
		"windows.LOCKFILE_FAIL_IMMEDIATELY",
		"windows.UnlockFileEx",
	} {
		if !strings.Contains(string(windowsSource), required) {
			t.Fatalf("Windows signed-pair lock is missing %q", required)
		}
	}
	otherSource, err := os.ReadFile("../oslock/oslock_other.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(otherSource), "locking is unsupported") ||
		strings.Contains(string(otherSource), "return nil, false, nil") {
		t.Fatal("unsupported platforms must fail closed instead of using a no-op lock")
	}
}
