package cli

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/selfupdate"
	"github.com/insajin/autopus-adk/pkg/version"
)

const (
	updateFreshnessReexecFlag     = "_freshness-reexec"
	updateFreshnessReexecEnv      = "AUTOPUS_UPDATE_FRESHNESS_REEXEC_NONCE"
	updateFreshnessNonceHexLength = 64
)

type updateFreshnessOutcome uint8

const (
	updateFreshnessProceed updateFreshnessOutcome = iota
	updateFreshnessStop
)

type updateFreshnessContextKey struct{}

type updateFreshnessDeps struct {
	currentVersion func() string
	checkLatest    func(string) (*selfupdate.ReleaseInfo, error)
	resolveBinary  func() (binaryPathInfo, error)
	install        func(*cobra.Command, string, *selfupdate.ReleaseInfo, binaryPathInfo) error
	reexec         func(*cobra.Command, string, []string, string) error
	invocationArgs func() []string
	newNonce       func() (string, error)
	lookupEnv      func(string) (string, bool)
	unsetEnv       func(string) error
}

type updateFreshnessGate struct {
	deps updateFreshnessDeps
}

func newUpdateFreshnessGate() updateFreshnessGate {
	return updateFreshnessGate{deps: updateFreshnessDeps{
		currentVersion: version.Version,
		checkLatest: func(current string) (*selfupdate.ReleaseInfo, error) {
			return selfupdate.NewChecker().CheckLatest(current, runtime.GOOS, runtime.GOARCH)
		},
		resolveBinary: resolveCurrentBinaryPath,
		install:       installFreshUpdateRelease,
		reexec:        reexecFreshUpdate,
		invocationArgs: func() []string {
			return append([]string(nil), os.Args[1:]...)
		},
		newNonce:  newUpdateFreshnessNonce,
		lookupEnv: os.LookupEnv,
		unsetEnv:  os.Unsetenv,
	}}
}

// @AX:WARN: [AUTO] enforce has at least eight if branches.
// @AX:REASON: [AUTO] Stable-version resolution, nonce consumption, manager ownership, signed installation, and one-shot re-exec must fail closed.
func (gate updateFreshnessGate) enforce(
	cmd *cobra.Command,
	preview bool,
	reexecNonce string,
) (updateFreshnessOutcome, error) {
	if updateFreshnessSatisfied(cmd.Context()) {
		return updateFreshnessProceed, nil
	}
	reexecuted := false
	if reexecNonce != "" {
		if err := gate.consumeReexecNonce(reexecNonce); err != nil {
			return updateFreshnessStop, err
		}
		reexecuted = true
	} else if _, exists := gate.deps.lookupEnv(updateFreshnessReexecEnv); exists {
		return updateFreshnessStop, fmt.Errorf(
			"update freshness re-exec nonce is missing its hidden flag",
		)
	}

	current, stable := stableCurrentVersion(gate.deps.currentVersion())
	if !stable {
		if reexecuted {
			return updateFreshnessStop, fmt.Errorf(
				"재실행된 auto binary version %q은 stable release가 아님",
				gate.deps.currentVersion(),
			)
		}
		markUpdateFreshnessSatisfied(cmd)
		return updateFreshnessProceed, nil
	}

	info, err := gate.deps.checkLatest(current)
	if err != nil {
		return updateFreshnessStop, fmt.Errorf("official latest release 확인 실패: %w", err)
	}
	if info == nil {
		markUpdateFreshnessSatisfied(cmd)
		return updateFreshnessProceed, nil
	}
	latest, stable := stableLatestVersion(info.TagName)
	if !stable || !selfupdate.IsNewerVersion(latest, current) {
		return updateFreshnessStop, fmt.Errorf(
			"official latest release tag %q is not a newer stable version",
			info.TagName,
		)
	}
	if reexecuted {
		return updateFreshnessStop, fmt.Errorf(
			"재실행된 auto binary v%s가 official latest release %s보다 오래됨",
			current,
			info.TagName,
		)
	}
	if preview {
		fmt.Fprintf(
			cmd.OutOrStdout(),
			"Freshness plan: auto binary v%s → %s, then one-shot re-exec before harness changes\n",
			current,
			info.TagName,
		)
		return updateFreshnessStop, nil
	}

	pathInfo, err := gate.deps.resolveBinary()
	if err != nil {
		return updateFreshnessStop, err
	}
	if pathInfo.IsManagerOwned() {
		return updateFreshnessStop, managerRequiredUpdateError(pathInfo.ManagedPath())
	}
	nonce, err := gate.deps.newNonce()
	if err != nil {
		return updateFreshnessStop, fmt.Errorf("update freshness re-exec nonce 생성 실패: %w", err)
	}
	if !validUpdateFreshnessNonce(nonce) {
		return updateFreshnessStop, fmt.Errorf("update freshness re-exec nonce 생성 결과가 유효하지 않음")
	}
	if err := gate.deps.install(cmd, current, info, pathInfo); err != nil {
		return updateFreshnessStop, err
	}

	args := appendUpdateFreshnessFlag(gate.deps.invocationArgs(), nonce)
	if err := gate.deps.reexec(cmd, pathInfo.ManagedPath(), args, nonce); err != nil {
		return updateFreshnessStop, fmt.Errorf("업데이트된 auto 재실행 실패: %w", err)
	}
	return updateFreshnessStop, nil
}

func (gate updateFreshnessGate) consumeReexecNonce(nonce string) error {
	if !validUpdateFreshnessNonce(nonce) {
		return fmt.Errorf("update freshness re-exec nonce 형식이 유효하지 않음")
	}
	expected, exists := gate.deps.lookupEnv(updateFreshnessReexecEnv)
	if !exists || len(expected) != len(nonce) ||
		subtle.ConstantTimeCompare([]byte(expected), []byte(nonce)) != 1 {
		return fmt.Errorf("update freshness re-exec nonce 검증 실패")
	}
	if err := gate.deps.unsetEnv(updateFreshnessReexecEnv); err != nil {
		return fmt.Errorf("update freshness re-exec nonce 소비 실패: %w", err)
	}
	return nil
}

func stableLatestVersion(raw string) (string, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) || strings.ContainsAny(raw, "-+") {
		return "", false
	}
	value := strings.TrimPrefix(raw, "v")
	stable, ok := stableCurrentVersion(value)
	return stable, ok && stable == value
}

func appendUpdateFreshnessFlag(args []string, nonce string) []string {
	flag := "--" + updateFreshnessReexecFlag + "=" + nonce
	out := make([]string, 0, len(args)+1)
	for index, arg := range args {
		if arg == "--" {
			out = append(out, flag)
			out = append(out, args[index:]...)
			return out
		}
		out = append(out, arg)
	}
	return append(out, flag)
}

func newUpdateFreshnessNonce() (string, error) {
	raw := make([]byte, updateFreshnessNonceHexLength/2)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func validUpdateFreshnessNonce(nonce string) bool {
	if len(nonce) != updateFreshnessNonceHexLength {
		return false
	}
	for _, char := range nonce {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func reexecFreshUpdate(
	parent *cobra.Command,
	binary string,
	args []string,
	nonce string,
) error {
	command := exec.CommandContext(parent.Context(), binary, args...) //nolint:gosec // canonical current binary path, passed without a shell
	command.Env = withEnvironmentValue(os.Environ(), updateFreshnessReexecEnv, nonce)
	command.Stdin = parent.InOrStdin()
	command.Stdout = parent.OutOrStdout()
	command.Stderr = parent.ErrOrStderr()
	return command.Run()
}

func updateFreshnessSatisfied(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	satisfied, _ := ctx.Value(updateFreshnessContextKey{}).(bool)
	return satisfied
}

func markUpdateFreshnessSatisfied(cmd *cobra.Command) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, updateFreshnessContextKey{}, true)
	cmd.SetContext(ctx)
	cmd.Root().SetContext(ctx)
}

type updateFreshnessNonceFlag struct {
	value string
	seen  bool
}

func (flag *updateFreshnessNonceFlag) Set(value string) error {
	if flag.seen {
		return fmt.Errorf("--%s는 한 번만 사용할 수 있습니다", updateFreshnessReexecFlag)
	}
	flag.seen = true
	if !validUpdateFreshnessNonce(value) {
		return fmt.Errorf("--%s nonce 형식이 유효하지 않습니다", updateFreshnessReexecFlag)
	}
	flag.value = value
	return nil
}

func (flag *updateFreshnessNonceFlag) String() string { return flag.value }
func (*updateFreshnessNonceFlag) Type() string        { return "nonce" }
