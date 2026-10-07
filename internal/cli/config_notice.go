package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// configNoticeFormat is the config notice of spec.md (SPEC-PANERM-001
// Compatibility Contract); %s is the pruned concrete paths joined with ", ".
const configNoticeFormat = "auto: warning: ignored removed autopus.yaml keys: %s; " +
	"the orchestra pane backend was retired (SPEC-PANERM-001); run \"auto update\" or delete the keys\n"

// configNotice turns the retired orchestra keys that config loads ignore into
// at most one stderr line per process (REQ-09). The loader only reports paths;
// this type decides whether and where they print. Reports that arrive before
// the executing command is known are held until bind, which writes them to the
// command's stderr unless its --quiet flag is set or that stderr is not a
// terminal. The line never goes to stdout and never changes the exit code.
type configNotice struct {
	mu         sync.Mutex
	isTerminal func(io.Writer) bool
	out        io.Writer
	pending    []string
	bound      bool
	silent     bool
	written    bool
}

func newConfigNotice(isTerminal func(io.Writer) bool) *configNotice {
	return &configNotice{isTerminal: isTerminal}
}

// report receives the paths one load pruned; config.SetRetiredKeyReporter
// installs it for the whole process.
func (n *configNotice) report(paths []string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.written || n.silent {
		return
	}
	n.pending = append(n.pending, paths...)
	if n.bound {
		n.flush()
	}
}

// bind fixes the executing command. The first call wins, so a nested root
// command that shares the context cannot reopen a silenced notice. A nil
// notice, as in tests that execute a root command without one, does nothing.
func (n *configNotice) bind(cmd *cobra.Command) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.bound {
		return
	}
	n.bound = true
	n.out = cmd.ErrOrStderr()
	quiet, _ := cmd.Flags().GetBool("quiet")
	if quiet || !n.isTerminal(n.out) {
		n.silent = true
		n.pending = nil
		return
	}
	n.flush()
}

func (n *configNotice) flush() {
	if len(n.pending) == 0 {
		return
	}
	paths := slices.Compact(slices.Sorted(slices.Values(n.pending)))
	n.pending = nil
	n.written = true
	_, _ = fmt.Fprintf(n.out, configNoticeFormat, terminalSafe(strings.Join(paths, ", ")))
}

// stderrIsTerminal reports whether w is a file attached to a terminal.
func stderrIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

type configNoticeContextKey struct{}

func withConfigNotice(ctx context.Context, notice *configNotice) context.Context {
	return context.WithValue(ctx, configNoticeContextKey{}, notice)
}

func configNoticeFromContext(ctx context.Context) *configNotice {
	if ctx == nil {
		return nil
	}
	notice, _ := ctx.Value(configNoticeContextKey{}).(*configNotice)
	return notice
}
