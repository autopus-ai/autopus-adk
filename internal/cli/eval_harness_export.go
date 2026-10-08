package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/evalregression"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Refusal reasons of `auto eval harness export` (SPEC-HARNEVAL-003 REQ-HR-03,
// REQ-HR-05). Each is the first word after "export refused:".
const (
	exportPrivateKeyMissing         = "private_key_missing"
	exportPrivateKeyInvalid         = "private_key_invalid"
	exportOutputExists              = "output_exists"
	exportOutputWriteFailed         = "output_write_failed"
	exportBindingFailed             = "binding_failed"
	exportReconstructionUnavailable = "reconstruction_unavailable"
	exportReportFailed              = "report_failed"
	exportSelfVerifyFailed          = "self_verify_failed"
)

// Signed evidence file names. auto check --eval-regression derives the
// attestation path from the report path, so --eval-regression-artifact alone
// selects both.
const (
	harnessEvidenceReport      = "eval_regression_report.json"
	harnessEvidenceAttestation = "eval_regression_attestation.json"
)

// harnessEvidenceMaxAge is the freshness window of the release check
// (REQ-HR-06); the self-verification before upload uses the same window.
const harnessEvidenceMaxAge = 72 * time.Hour

// harnessChildEnvNames is the whole environment a child of the signer may
// receive: tool lookup, home and temp directories, locale, and the Go
// toolchain locations a baseline driver build reads. A credential, token,
// OIDC request variable, or the signing key never has one of these names.
var harnessChildEnvNames = map[string]bool{
	"PATH": true, "HOME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "TZ": true,
	"GOROOT": true, "GOPATH": true, "GOMODCACHE": true, "GOCACHE": true, "GOTOOLCHAIN": true,
}

// harnessChildEnv keeps the entries of parent whose name is allowlisted, in
// order. Every child process the export starts gets exactly this slice.
func harnessChildEnv(parent []string) []string {
	env := []string{}
	for _, entry := range parent {
		if name, _, found := strings.Cut(entry, "="); found && harnessChildEnvNames[name] {
			env = append(env, entry)
		}
	}
	return env
}

// harnessExportRequest is what the trusted reconstruction of REQ-HR-02 and
// REQ-HR-09 reads: the main checkout, the downloaded unsigned artifact, the
// run meta file, the binding computed from the checkout, and the allowlisted
// environment for every process it starts.
type harnessExportRequest struct {
	Root, Input, RunMeta string
	Binding              harneval.Binding
	BindingDigest        string
	Env                  []string
}

// harnessSignable is a session the reconstruction accepted: its documents
// match the session_result attestation, its protocol the trusted one, and its
// records the re-derived outcomes. SignedTaskFloor is main's
// floors.signed_agent_tasks.
type harnessSignable struct {
	Session         *harneval.Session
	SignedTaskFloor int
}

// harnessExportSeams are the export's seams; the zero value is production.
// A nil reconstruct is the trusted reconstruction (eval_harness_reconstruct.go)
// over sources, whose nil members refuse, because nothing unverified is
// signed. afterReport runs between the two evidence files.
type harnessExportSeams struct {
	reconstruct func(context.Context, harnessExportRequest) (harnessSignable, error)
	sources     harnessTrustSources
	binding     func(context.Context, string, harneval.BindingOptions) (harneval.Binding, error)
	trusted     func() map[string]ed25519.PublicKey
	environ     func() []string
	now         func() time.Time
	afterReport func() error
}

func (s harnessExportSeams) withDefaults() harnessExportSeams {
	if s.reconstruct == nil {
		s.reconstruct = s.sources.reconstruct
	}
	if s.binding == nil {
		s.binding = harneval.ComputeBinding
	}
	if s.trusted == nil {
		s.trusted = evalregression.CommittedEvalRegressionPublicKeys
	}
	if s.environ == nil {
		s.environ = os.Environ
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func exportRefusal(reason, detail string) error { return errors.New(reason + ": " + detail) }

const evalHarnessExportLong = `Sign one verified live session as eval_regression evidence (SPEC-HARNEVAL-003).

The signing key is read from stdin only, as base64 of a 64-byte ed25519 key;
there is no key flag. Run it as
  printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY auto eval harness export ...
so the key stays out of the environment of auto and of every child process,
which receive an allowlisted environment.

--output names a directory that must not exist; it is created with mode 0700
and holds eval_regression_report.json and eval_regression_attestation.json,
mode 0600. The evidence is verified against the committed allowlist before the
command succeeds; any failure removes the directory and exits 1.`

// newEvalHarnessExportCmd is the production `auto eval harness export`.
func newEvalHarnessExportCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	return newEvalHarnessExportCmdWith(deps, dir, harnessExportSeams{})
}

func newEvalHarnessExportCmdWith(deps evalHarnessDeps, dir *string, seams harnessExportSeams) *cobra.Command {
	var input, runMeta, output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Sign a verified live session as eval_regression evidence (key on stdin)",
		Long:  evalHarnessExportLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if input == "" || runMeta == "" || output == "" {
				return errors.New("--input, --run-meta, and --output are required")
			}
			export := harnessExport{deps: deps, seams: seams.withDefaults(), root: *dir, input: input, runMeta: runMeta, output: output}
			line, err := export.run(cmd.Context(), cmd.InOrStdin())
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "harness-eval: export refused: "+intakePrintable(err.Error()))
				return harnessFailure()
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			fmt.Fprintln(cmd.ErrOrStderr(), "harness-eval: signed evidence written to "+intakePrintable(output))
			return nil
		},
	}
	cmd.Flags().StringVar(&input, "input", "", "directory of the downloaded unsigned live result")
	cmd.Flags().StringVar(&runMeta, "run-meta", "", "file holding {run_id, run_attempt, run_created_at, attempt_started_at}")
	cmd.Flags().StringVar(&output, "output", "", "directory to create for the signed evidence; it must not exist")
	return cmd
}

// harnessExport is one export invocation.
type harnessExport struct {
	deps                 evalHarnessDeps
	seams                harnessExportSeams
	root, input, runMeta string
	output               string
}

// run reads the key, then checks the output path, computes the trusted
// binding, has the reconstruction verify the session, signs, writes, and
// self-verifies. It returns the strict check line of the written evidence.
func (e harnessExport) run(ctx context.Context, stdin io.Reader) (string, error) {
	key, err := readExportKey(stdin)
	if err != nil {
		return "", err
	}
	defer clear(key)
	if _, err := os.Lstat(e.output); err == nil {
		return "", exportRefusal(exportOutputExists, e.output)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", exportRefusal(exportOutputWriteFailed, err.Error())
	}
	env := harnessChildEnv(e.seams.environ())
	binding, err := e.seams.binding(ctx, e.root, harneval.BindingOptions{Adapters: e.deps.run.Adapters, Env: env})
	if err != nil {
		return "", exportRefusal(exportBindingFailed, err.Error())
	}
	digest := binding.Digest()
	signable, err := e.seams.reconstruct(ctx, harnessExportRequest{
		Root: e.root, Input: e.input, RunMeta: e.runMeta, Binding: binding, BindingDigest: digest, Env: env,
	})
	if err != nil {
		return "", err
	}
	reportBytes, attBytes, err := signHarnessEvidence(signable, digest, key)
	if err != nil {
		return "", err
	}
	if err := e.writeEvidence(reportBytes, attBytes); err != nil {
		return "", err
	}
	decision := evaluateEvalRegressionStrict(filepath.Join(e.output, harnessEvidenceReport),
		filepath.Join(e.output, harnessEvidenceAttestation), harnessEvidenceMaxAge, e.seams.now(), e.seams.trusted(),
		harneval.LanePolicy(digest))
	if decision.Reason != "ok" && decision.Reason != "regression_blocked" {
		_ = os.RemoveAll(e.output)
		return "", exportRefusal(exportSelfVerifyFailed, decision.Reason)
	}
	return "eval-regression: " + decision.Reason + " (version=" + decision.AttributedVersion + ")", nil
}

// readExportKey reads the signing key from stdin, its only source. An empty
// stdin is private_key_missing before readPrivateKey runs; any other defect,
// including a public half its seed does not derive, is private_key_invalid.
func readExportKey(stdin io.Reader) (ed25519.PrivateKey, error) {
	raw, err := io.ReadAll(io.LimitReader(stdin, maxEncodedPrivateKeyBytes+1))
	defer clear(raw)
	if err != nil {
		return nil, exportRefusal(exportPrivateKeyInvalid, "stdin could not be read")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, exportRefusal(exportPrivateKeyMissing, "stdin holds no signing key")
	}
	key, err := readPrivateKey(bytes.NewReader(raw))
	if err != nil {
		return nil, exportRefusal(exportPrivateKeyInvalid, "stdin holds no base64 64-byte ed25519 key")
	}
	seed := key.Seed()
	derived := ed25519.NewKeyFromSeed(seed)
	consistent := subtle.ConstantTimeCompare(derived, key) == 1
	clear(seed)
	clear(derived)
	if !consistent {
		clear(key)
		return nil, exportRefusal(exportPrivateKeyInvalid, "the key's public half is not the one its seed derives")
	}
	return key, nil
}

// signHarnessEvidence judges the session with the SPEC-HARNEVAL-001 verdict
// and the signed-lane task floor, maps it into the report of binding digest,
// and signs the report bytes with the harness lane context.
func signHarnessEvidence(signable harnessSignable, digest string, key ed25519.PrivateKey) ([]byte, []byte, error) {
	session := signable.Session
	if session == nil {
		return nil, nil, exportRefusal(exportReportFailed, "the reconstruction returned no session")
	}
	verdict, err := harneval.SignedLaneVerdict(session, signable.SignedTaskFloor)
	if err != nil {
		return nil, nil, exportRefusal(exportReportFailed, err.Error())
	}
	report, err := harneval.BuildReportV1(session.Protocol, verdict, digest)
	if err != nil {
		return nil, nil, exportRefusal(exportReportFailed, err.Error())
	}
	reportBytes, err := harneval.EncodeReportV1(report)
	if err != nil {
		return nil, nil, exportRefusal(exportReportFailed, err.Error())
	}
	att, err := evalregression.SignEvalRegressionAttestationV2(reportBytes, harneval.LanePolicy(digest), key)
	if err != nil {
		return nil, nil, exportRefusal(exportReportFailed, err.Error())
	}
	attBytes, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		return nil, nil, exportRefusal(exportReportFailed, err.Error())
	}
	return reportBytes, append(attBytes, '\n'), nil
}

// writeEvidence creates the output directory with mode 0700 and both files
// exclusively with mode 0600. Success needs both files; any failure removes
// the directory, which this run created.
func (e harnessExport) writeEvidence(report, attestation []byte) (err error) {
	if err := os.Mkdir(e.output, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return exportRefusal(exportOutputExists, e.output)
		}
		return exportRefusal(exportOutputWriteFailed, err.Error())
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(e.output)
		}
	}()
	if err := os.Chmod(e.output, 0o700); err != nil {
		return exportRefusal(exportOutputWriteFailed, err.Error())
	}
	if err := writeEvidenceFile(filepath.Join(e.output, harnessEvidenceReport), report); err != nil {
		return exportRefusal(exportOutputWriteFailed, err.Error())
	}
	if e.seams.afterReport != nil {
		if err := e.seams.afterReport(); err != nil {
			return exportRefusal(exportOutputWriteFailed, err.Error())
		}
	}
	if err := writeEvidenceFile(filepath.Join(e.output, harnessEvidenceAttestation), attestation); err != nil {
		return exportRefusal(exportOutputWriteFailed, err.Error())
	}
	return nil
}

// writeEvidenceFile creates path exclusively with mode 0600 and syncs it.
func writeEvidenceFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Chmod(0o600)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	return errors.Join(writeErr, file.Close())
}
