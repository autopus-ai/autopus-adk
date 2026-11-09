// Command canaryhelper is the one program the release prep may run as root
// without a password. Every privileged step of the local live canary goes
// through it: the isolation root, the one-shot canary account, staging the
// project in and out, ownership changes, and the privilege drop itself.
//
// It is installed root-owned under /Library/PrivilegedHelperTools and named
// in a sudoers rule, so any process of the operator's account can call it.
// It therefore trusts none of its arguments: every path must be the fixed
// isolation-root pattern, every account is derived from the run nonce and
// verified by its marker, it never executes a caller-supplied binary as root,
// and it reads or writes the operator's files only from a child running as
// the operator.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "release canary helper: %v\n", err)
		os.Exit(125)
	}
}
