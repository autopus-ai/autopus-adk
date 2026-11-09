package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	productionSudoPath = "/usr/bin/sudo"
	productionEnvPath  = "/usr/bin/env"
	productionUser     = "nobody"
	// productionHelperPath runs a local release prep's smoke as nobody; the
	// operator's sudoers rule allows only this file without a password.
	productionHelperPath = "/Library/PrivilegedHelperTools/co.autopus.release-canary"
)

// releasePrepRoot is the isolation root a local release prep creates through
// the helper. A CI-materialized root has another name and keeps sudo -u.
var releasePrepRoot = regexp.MustCompile(`^/private/tmp/autopus-adk-release-prep-([0-9a-f]{32})-final$`)

type canaryUIDIsolationPolicy struct {
	runnerPath               string
	envPath                  string
	user                     string
	expectedUID              uint32
	expectedGIDs             map[uint32]struct{}
	runnerOwnerUID           uint32
	requireProductionRunner  bool
	requireArtifactOtherExec bool
}

type canaryUIDIsolation struct {
	root, home, temporary string
	policy                canaryUIDIsolationPolicy
	// helperNonce is set for a release-prep root: the smoke then runs through
	// the helper's run-nobody instead of sudo -u nobody.
	helperNonce string
}

func productionCanaryUIDIsolationPolicy() (canaryUIDIsolationPolicy, error) {
	account, err := user.Lookup(productionUser)
	if err != nil || account.Username != productionUser {
		return canaryUIDIsolationPolicy{}, errors.New("nobody release identity is unavailable")
	}
	uid, err := strconv.ParseUint(account.Uid, 10, 32)
	if err != nil {
		return canaryUIDIsolationPolicy{}, errors.New("nobody release UID is invalid")
	}
	groupIDs, err := account.GroupIds()
	if err != nil || len(groupIDs) == 0 {
		return canaryUIDIsolationPolicy{}, errors.New("nobody release groups are unavailable")
	}
	groups := make(map[uint32]struct{}, len(groupIDs)+1)
	for _, groupID := range append(groupIDs, account.Gid) {
		value, parseErr := strconv.ParseUint(groupID, 10, 32)
		if parseErr != nil {
			return canaryUIDIsolationPolicy{}, errors.New("nobody release group is invalid")
		}
		groups[uint32(value)] = struct{}{}
	}
	return canaryUIDIsolationPolicy{
		runnerPath: productionSudoPath, envPath: productionEnvPath, user: productionUser,
		expectedUID: uint32(uid), expectedGIDs: groups, runnerOwnerUID: 0,
		requireProductionRunner: true, requireArtifactOtherExec: true,
	}, nil
}

func validateCanaryUIDIsolation(
	root, ompExecutable, artifact string,
	policy canaryUIDIsolationPolicy,
) (*canaryUIDIsolation, error) {
	isolation, err := validateCanaryUIDIsolationRoot(root, ompExecutable, policy)
	if err != nil {
		return nil, err
	}
	if policy.requireArtifactOtherExec &&
		validateArtifactUIDAccess(artifact, policy.expectedUID, policy.expectedGIDs, policy.runnerOwnerUID) != nil {
		return nil, errors.New("signed artifact or parent path is unsafe for nobody execution")
	}
	return isolation, nil
}

func validateCanaryUIDIsolationRoot(
	root, ompExecutable string,
	policy canaryUIDIsolationPolicy,
) (*canaryUIDIsolation, error) {
	if policy.user != productionUser || policy.runnerPath == "" || policy.envPath == "" {
		return nil, errors.New("release canary UID isolation policy is invalid")
	}
	if policy.requireProductionRunner &&
		(policy.runnerPath != productionSudoPath || policy.envPath != productionEnvPath || policy.runnerOwnerUID != 0) {
		return nil, errors.New("release canary privilege runner is not production-pinned")
	}
	if err := validateCanaryRunner(policy.runnerPath, policy.runnerOwnerUID, true); err != nil {
		return nil, err
	}
	if err := validateCanaryRunner(policy.envPath, policy.runnerOwnerUID, false); err != nil {
		return nil, err
	}
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("release canary root must be an absolute canonical path")
	}
	if err := validateCanaryPath(root, true, 0o755, policy.runnerOwnerUID); err != nil {
		return nil, errors.New("release canary root ownership or mode is unsafe")
	}
	for _, name := range []string{"home", "tmp"} {
		if err := validateCanaryPath(filepath.Join(root, name), true, 0o700, policy.expectedUID); err != nil {
			return nil, errors.New("release canary private directory ownership or mode is unsafe")
		}
	}
	if ompExecutable != filepath.Join(root, "omp-darwin-arm64") {
		return nil, errors.New("release canary OMP path is outside the isolated root")
	}
	if err := validateCanaryPath(ompExecutable, false, 0o555, policy.runnerOwnerUID); err != nil {
		return nil, errors.New("release canary OMP ownership or mode is unsafe")
	}
	isolation := &canaryUIDIsolation{
		root: root, home: filepath.Join(root, "home"), temporary: filepath.Join(root, "tmp"), policy: policy,
	}
	if match := releasePrepRoot.FindStringSubmatch(root); match != nil && policy.requireProductionRunner {
		if err := validateCanaryRunner(productionHelperPath, 0, false); err != nil {
			return nil, err
		}
		isolation.helperNonce = match[1]
	}
	return isolation, nil
}

func validateCanaryPath(path string, directory bool, mode os.FileMode, ownerUID uint32) error {
	info, err := os.Lstat(path)
	uid, uidErr := canaryFileUID(info)
	if err != nil || uidErr != nil || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != mode || uid != ownerUID || directory != info.IsDir() ||
		!directory && !info.Mode().IsRegular() {
		return errors.New("release canary path identity is unsafe")
	}
	return nil
}

func validateCanaryRunner(path string, ownerUID uint32, requireSetUID bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("release canary privilege runner path is unsafe")
	}
	info, err := os.Lstat(path)
	uid, uidErr := canaryFileUID(info)
	if err != nil || uidErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 || uid != ownerUID ||
		requireSetUID && info.Mode()&os.ModeSetuid == 0 {
		return errors.New("release canary privilege runner is unavailable or unsafe")
	}
	return nil
}

func validateArtifactUIDAccess(
	path string,
	targetUID uint32,
	targetGIDs map[uint32]struct{},
	trustedOwnerUID uint32,
) error {
	info, err := os.Lstat(path)
	uid, _, identityErr := canaryFileIdentity(info)
	if err != nil || identityErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		uid == targetUID || !canaryTargetMode(info, targetUID, targetGIDs, 0o400, 0o040, 0o004) ||
		!canaryTargetMode(info, targetUID, targetGIDs, 0o100, 0o010, 0o001) ||
		canaryTargetMode(info, targetUID, targetGIDs, 0o200, 0o020, 0o002) {
		return errors.New("artifact identity or target access mode is unsafe")
	}
	return validateArtifactAncestorUIDAccess(filepath.Dir(path), targetUID, targetGIDs, trustedOwnerUID)
}

func validateArtifactAncestorUIDAccess(
	path string,
	targetUID uint32,
	targetGIDs map[uint32]struct{},
	trustedOwnerUID uint32,
) error {
	for parent := path; ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		uid, _, identityErr := canaryFileIdentity(info)
		if err != nil || identityErr != nil || uid == targetUID || !info.IsDir() ||
			info.Mode()&os.ModeSymlink != 0 ||
			!canaryTargetMode(info, targetUID, targetGIDs, 0o100, 0o010, 0o001) {
			return errors.New("artifact parent is not immutable and traversable by nobody")
		}
		if canaryTargetMode(info, targetUID, targetGIDs, 0o200, 0o020, 0o002) &&
			(uid != trustedOwnerUID || info.Mode()&os.ModeSticky == 0) {
			return errors.New("artifact parent is not immutable and traversable by nobody")
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return nil
}

func canaryTargetMode(
	info os.FileInfo,
	targetUID uint32,
	targetGIDs map[uint32]struct{},
	ownerMode, groupMode, otherMode os.FileMode,
) bool {
	uid, gid, err := canaryFileIdentity(info)
	if err != nil {
		return false
	}
	if uid == targetUID {
		return info.Mode().Perm()&ownerMode != 0
	}
	if _, ok := targetGIDs[gid]; ok {
		return info.Mode().Perm()&groupMode != 0
	}
	return info.Mode().Perm()&otherMode != 0
}

func newCanaryCommand(
	ctx context.Context,
	artifact string,
	isolation *canaryUIDIsolation,
	args ...string,
) (*exec.Cmd, error) {
	if isolation == nil {
		return exec.CommandContext(ctx, artifact, args...), nil
	}
	if isolation.helperNonce != "" {
		// The helper runs only the candidate it installed in this root and
		// sets the environment itself, so only arguments go through.
		if artifact != filepath.Join(isolation.root, "auto") {
			return nil, errors.New("a release-prep smoke runs only the candidate installed in its root")
		}
		helperArgs := append([]string{"-n", productionHelperPath, "run-nobody", isolation.helperNonce, "--"}, args...)
		return exec.CommandContext(ctx, isolation.policy.runnerPath, helperArgs...), nil
	}
	commandArgs := []string{"-n", "-u", isolation.policy.user, isolation.policy.envPath, "-i",
		"HOME=" + isolation.home, "TMPDIR=" + isolation.temporary,
		"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", artifact}
	commandArgs = append(commandArgs, args...)
	return exec.CommandContext(ctx, isolation.policy.runnerPath, commandArgs...), nil
}
