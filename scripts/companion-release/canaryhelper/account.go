//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The one-shot canary identity, derived exactly as prepare-release-user-lib.sh
// derives it, so the shell and the helper always name the same account.
const (
	canaryUIDMin      = 50000
	canaryUIDMax      = 59999
	canaryUIDAttempts = 64
	canaryGID         = 20
	dscl              = "/usr/bin/dscl"
)

type canaryAccount struct {
	name, marker, home string
	uid                int
}

func (r *run) account(attemptText string) (canaryAccount, error) {
	attempt, err := canonicalID(attemptText)
	if err != nil || attempt < 1 || attempt > canaryUIDAttempts {
		return canaryAccount{}, fmt.Errorf("attempt must be 1 to %d", canaryUIDAttempts)
	}
	seed, err := strconv.ParseUint(r.nonce[:8], 16, 32)
	if err != nil {
		return canaryAccount{}, err
	}
	span := uint64(canaryUIDMax - canaryUIDMin + 1)
	uid := canaryUIDMin + int((seed+uint64(attempt)-1)%span)
	return canaryAccount{
		name:   fmt.Sprintf("_autopus_v110_%s_%02d", r.nonce[:8], attempt),
		marker: fmt.Sprintf("Autopus Release Canary %s attempt %02d uid %d", r.nonce, attempt, uid),
		home:   r.home,
		uid:    uid,
	}, nil
}

func (r *run) accountCommand(command string, rest []string) error {
	if len(rest) < 1 {
		return errors.New(usage)
	}
	a, err := r.account(rest[0])
	if err != nil {
		return err
	}
	switch {
	case command == "account-create" && len(rest) == 1:
		return r.createAccount(a)
	case command == "account-kill" && len(rest) == 2 && (rest[1] == "TERM" || rest[1] == "KILL"):
		if err := verifyOwned(a); err != nil {
			return err
		}
		return killUID(a.uid, rest[1])
	case command == "account-delete" && len(rest) == 1:
		if err := verifyOwned(a); err != nil {
			return err
		}
		if free, err := uidProcessFree(a.uid); err != nil || !free {
			return errors.New("the canary account still runs processes")
		}
		return runQuiet(dscl, ".", "-delete", "/Users/"+a.name)
	}
	return errors.New(usage)
}

// createAccount makes the hidden, login-less account only when neither its
// name nor its UID is taken and no process runs under that UID.
func (r *run) createAccount(a canaryAccount) error {
	if err := r.checkRoot(); err != nil {
		return err
	}
	inventory, err := accountInventory()
	if err != nil {
		return err
	}
	for name, uid := range inventory {
		if name == a.name || uid == a.uid {
			return errors.New("the canary account name or UID is already taken")
		}
	}
	if free, err := uidProcessFree(a.uid); err != nil || !free {
		return errors.New("a process already runs under the canary UID")
	}
	path := "/Users/" + a.name
	// The marker goes first: once it is set, the account is provably ours and
	// later steps may finish or remove it.
	for _, attr := range [][2]string{
		{"RealName", a.marker},
		{"UniqueID", strconv.Itoa(a.uid)},
		{"PrimaryGroupID", strconv.Itoa(canaryGID)},
		{"NFSHomeDirectory", a.home},
		{"UserShell", "/usr/bin/false"},
		{"IsHidden", "1"},
	} {
		if err := runQuiet(dscl, ".", "-create", path, attr[0], attr[1]); err != nil {
			return fmt.Errorf("set %s on the canary account: %w", attr[0], err)
		}
	}
	return verifyOwned(a)
}

// verifyOwned checks the account carries this run's marker and is the only
// account with its UID, so the helper never acts on someone else's account.
func verifyOwned(a canaryAccount) error {
	for attr, want := range map[string]string{"RealName": a.marker, "UniqueID": strconv.Itoa(a.uid)} {
		got, err := accountValue(a.name, attr)
		if err != nil || got != want {
			return errors.New("the canary account is missing or not owned by this run")
		}
	}
	inventory, err := accountInventory()
	if err != nil {
		return err
	}
	matches := 0
	for name, uid := range inventory {
		if uid == a.uid {
			if name != a.name {
				return errors.New("another account shares the canary UID")
			}
			matches++
		}
	}
	if matches != 1 {
		return errors.New("the canary account is not exclusively owned")
	}
	return nil
}

func accountValue(name, attribute string) (string, error) {
	out, err := exec.Command(dscl, ".", "-read", "/Users/"+name, attribute).Output()
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(out))
	if _, value, ok := strings.Cut(text, attribute+":"); ok {
		return strings.Join(strings.Fields(value), " "), nil
	}
	return "", errors.New("attribute missing")
}

func accountInventory() (map[string]int, error) {
	out, err := exec.Command(dscl, ".", "-list", "/Users", "UniqueID").Output()
	if err != nil {
		return nil, errors.New("cannot list accounts")
	}
	inventory := map[string]int{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, errors.New("account inventory is malformed")
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, errors.New("account inventory is malformed")
		}
		inventory[fields[0]] = uid
	}
	return inventory, nil
}

func uidProcessFree(uid int) (bool, error) {
	err := exec.Command("/usr/bin/pgrep", "-u", strconv.Itoa(uid)).Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return false, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == 1:
		return true, nil
	}
	return false, errors.New("cannot inspect processes")
}

func killUID(uid int, signal string) error {
	err := exec.Command("/usr/bin/pkill", "-"+signal, "-u", strconv.Itoa(uid)).Run()
	var exitErr *exec.ExitError
	if err != nil && !(errors.As(err, &exitErr) && exitErr.ExitCode() == 1) {
		return fmt.Errorf("signal canary processes: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func runQuiet(name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
