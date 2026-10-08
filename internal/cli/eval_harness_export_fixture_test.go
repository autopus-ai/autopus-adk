package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/evalregression"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

const (
	exportSessionID = "4f9c2e7a1b3d5f60718293a4b5c6d7e8"
	exportStartedAt = "2026-10-06T21:18:22+09:00"
	exportTask      = "GT-AG-001"
)

// exportBinding is a fixed binding; the export signs for its digest.
func exportBinding() harneval.Binding {
	return harneval.Binding{SchemaVersion: harneval.BindingSchemaV1, Model: "gpt-test", BaselineRef: "v0.50.122",
		BaselineCommit: strings.Repeat("c", 40), SigningKeyID: evalregression.ADKHarnessEvalKeyID}
}

// exportSession is one task at K=2 whose baseline passes twice and whose
// candidate trials have the given outcomes; [pass, fail] is the S4 normal
// session.
func exportSession(candidate ...string) *harneval.Session {
	phase := harneval.CalibrationPhase{Status: harneval.CalibrationPassed,
		Tasks: []harneval.CalibrationTask{{TaskID: exportTask, CleanAccepted: true}}}
	after := phase
	session := &harneval.Session{
		Protocol: harneval.Protocol{SessionID: exportSessionID, StartedAt: exportStartedAt, BaselineRef: "v0.50.122",
			Calibration: phase, Policy: harneval.LivePolicy{K: 2, ThresholdBP: -1000, CompletenessFloor: 0.9}},
		Calibration: &harneval.Calibration{SessionID: exportSessionID, Before: phase, After: &after},
	}
	for trial, outcome := range candidate {
		for _, arm := range []string{harneval.ArmBaseline, harneval.ArmCandidate} {
			attempt := harneval.Attempt{TaskID: exportTask, Arm: arm, Trial: trial}
			record := harneval.Record{SessionID: exportSessionID, TaskID: exportTask, Arm: arm, Trial: trial,
				Outcome: harneval.OutcomePass, Signal: "accepted", Oracle: &harneval.OracleObservation{Ran: true, ExpectedPassed: 1}}
			if arm == harneval.ArmCandidate && outcome == "fail" {
				record.Outcome, record.Signal = harneval.OutcomeFail, "oracle_failed"
				record.Oracle.ExpectedPassed, record.Oracle.ExpectedFailed = 0, 1
			}
			session.Protocol.Order = append(session.Protocol.Order, attempt)
			session.Records = append(session.Records, record)
		}
	}
	return session
}

func exportStarted(t *testing.T) time.Time {
	t.Helper()
	started, err := time.Parse(time.RFC3339, exportStartedAt)
	require.NoError(t, err)
	return started
}

// exportKey is a test key made in the test and its stdin encoding.
func exportKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv, base64.StdEncoding.EncodeToString(priv)
}

// exportSeams signs session with the binding stub, trusts pub, and runs 30
// hours after started_at: approval latency has no upper bound (S4).
func exportSeams(t *testing.T, session *harneval.Session, pub ed25519.PublicKey) harnessExportSeams {
	started := exportStarted(t)
	return harnessExportSeams{
		binding: func(context.Context, string, harneval.BindingOptions) (harneval.Binding, error) {
			return exportBinding(), nil
		},
		reconstruct: func(context.Context, harnessExportRequest) (harnessSignable, error) {
			return harnessSignable{Session: session}, nil
		},
		trusted: func() map[string]ed25519.PublicKey {
			return map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: pub}
		},
		environ: func() []string { return []string{"PATH=/usr/bin"} },
		now:     func() time.Time { return started.Add(30 * time.Hour) },
	}
}

// runExport executes `auto eval harness export` in-process with stdin.
func runExport(t *testing.T, seams harnessExportSeams, stdin string, args ...string) harnessOutcome {
	t.Helper()
	dir := t.TempDir()
	cmd := newEvalHarnessExportCmdWith(evalHarnessDeps{}, &dir, seams)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true // as the auto root sets them
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	code := 0
	if err != nil {
		code = exitCodeForError(err)
		if !isJSONFatalError(err) {
			stderr.WriteString("Error: " + err.Error() + "\n")
		}
	}
	return harnessOutcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func exportArgs(output string) []string {
	return []string{"--input", "unsigned", "--run-meta", "run-meta.json", "--output", output}
}

// keyEncodings are the S2 forms of the 64-byte key and its 32-byte seed.
func keyEncodings(priv ed25519.PrivateKey) [][]byte {
	forms := [][]byte{priv, priv.Seed()}
	for _, raw := range [][]byte{priv, priv.Seed()} {
		forms = append(forms, []byte(base64.StdEncoding.EncodeToString(raw)), []byte(base64.RawStdEncoding.EncodeToString(raw)),
			[]byte(base64.URLEncoding.EncodeToString(raw)), []byte(base64.RawURLEncoding.EncodeToString(raw)),
			[]byte(hex.EncodeToString(raw)), []byte(strings.ToUpper(hex.EncodeToString(raw))))
	}
	return append(forms, []byte(base64.RawStdEncoding.EncodeToString(priv.Seed())[:40]))
}

func assertNoKey(t *testing.T, priv ed25519.PrivateKey, where string, data []byte) {
	t.Helper()
	for _, form := range keyEncodings(priv) {
		assert.False(t, bytes.Contains(data, form), "%s holds a form of the signing key", where)
	}
}
