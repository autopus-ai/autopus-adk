//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
)

const usage = "usage: canaryhelper prepare-root NONCE | remove-root NONCE | install-exec NONCE auto|omp-darwin-arm64 SOURCE |" +
	" stage NONCE SOURCE | unstage NONCE DESTINATION | chown NONCE canary|nobody ATTEMPT |" +
	" account-create NONCE ATTEMPT | account-kill NONCE ATTEMPT TERM|KILL | account-delete NONCE ATTEMPT |" +
	" run-canary NONCE ATTEMPT -- ARGV... | run-nobody NONCE -- ARGS..."

func dispatch(args []string) error {
	if len(args) > 0 && (args[0] == packCommand || args[0] == unpackCommand || args[0] == readCommand) {
		return childMode(args) // runs as the operator, started by the helper itself
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return errors.New("run through sudo")
	}
	op, err := callerIdentity()
	if err != nil {
		return err
	}
	if len(args) < 2 {
		return errors.New(usage)
	}
	r, err := newRun(args[1])
	if err != nil {
		return err
	}
	rest := args[2:]
	// Every command but prepare-root works on an existing root; take its lock
	// first so no two helper calls on one root interleave.
	if args[0] != "prepare-root" {
		if _, err := os.Lstat(r.root); err == nil {
			if err := r.checkRoot(); err != nil {
				return err
			}
			if err := r.lock(); err != nil {
				return err
			}
		}
	}
	switch args[0] {
	case "prepare-root":
		return need(rest, 0, r.prepareRoot)
	case "remove-root":
		return need(rest, 0, r.removeRoot)
	case "install-exec":
		if len(rest) != 2 {
			return errors.New(usage)
		}
		return r.installExec(op, rest[0], rest[1])
	case "stage":
		if len(rest) != 1 {
			return errors.New(usage)
		}
		return r.stage(op, rest[0])
	case "unstage":
		if len(rest) != 1 {
			return errors.New(usage)
		}
		return r.unstage(op, rest[0])
	case "chown":
		return r.chownCommand(rest)
	case "account-create", "account-kill", "account-delete":
		return r.accountCommand(args[0], rest)
	case "run-canary":
		if len(rest) < 3 || rest[1] != "--" {
			return errors.New(usage)
		}
		a, err := r.account(rest[0])
		if err != nil {
			return err
		}
		return r.runCanary(a, rest[2:])
	case "run-nobody":
		if len(rest) < 1 || rest[0] != "--" {
			return errors.New(usage)
		}
		return r.runNobody(rest[1:])
	}
	return fmt.Errorf("unknown command %q; %s", args[0], usage)
}

func need(rest []string, n int, f func() error) error {
	if len(rest) != n {
		return errors.New(usage)
	}
	return f()
}
