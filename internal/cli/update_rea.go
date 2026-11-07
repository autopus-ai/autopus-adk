package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reaAnswerLimit bounds the REA answer kept from one input line.
const reaAnswerLimit = 64

// resolveUpdateRea resolves an update run's REA decision before the preview
// branch (Update Prompt Contract): the flag, else the Decision State
// Contract. The preview, the existing platforms, and the newly detected ones
// all read the action it sets. A preview never reaches finish, so it prints
// the hint of an undecided project and the OMP·OpenCode line here.
func resolveUpdateRea(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, on, off, preview bool) *reaRun {
	run := newReaRun(dir, cfg, cfg.Features.Rea.Enabled != nil)
	state := reaFlagState(on, off)
	if state == reaUndecided {
		state = resolveReaDecision(dir, cfg).State
	}
	out := cmd.OutOrStdout()
	run.decide(reaContext(cmd), out, cfg, state)
	if preview {
		if state == reaUndecided && run.active {
			run.line(out, run.text.Hint)
		}
		run.guidance(out, cfg.Platforms)
	}
	return run
}

// prepare runs before an update's platform writes. It snapshots the target
// files of the configured and newly detected platforms, then settles an
// undecided update: a run the compose gate treats as interactive asks the REA
// question once, default No, and applies the answer in this run; every other
// run reads no stdin and prints the hint. Enter, or an answer that is neither
// yes nor no, leaves REA undecided and autopus.yaml untouched, so the default
// registers nothing.
func (r *reaRun) prepare(cmd *cobra.Command, cfg *config.HarnessConfig, gate composeGate) {
	r.snapshot(append(append([]string{}, cfg.Platforms...), gate.Detected...))
	if r.state != reaUndecided || !r.active {
		return
	}
	out := cmd.OutOrStdout()
	if gate.mode() != composeModeAsk {
		r.line(out, r.text.Hint)
		return
	}
	r.line(out, r.text.Question)
	switch strings.ToLower(readReaAnswer(cmd.InOrStdin())) {
	case "y", "yes":
		r.decide(reaContext(cmd), out, cfg, reaOn)
	case "n", "no":
		r.decide(reaContext(cmd), out, cfg, reaOff)
	default:
		r.line(out, r.text.Hint)
	}
}

// readReaAnswer reads one answer line a byte at a time, so the answers the
// composition step reads next stay queued in the input. Input end reads as
// Enter.
func readReaAnswer(in io.Reader) string {
	var line []byte
	buf := make([]byte, 1)
	for empty := 0; empty < 100; {
		n, err := in.Read(buf)
		if n == 0 && err == nil {
			empty++
			continue
		}
		if n > 0 && buf[0] == '\n' {
			break
		}
		if n > 0 && len(line) < reaAnswerLimit {
			line = append(line, buf[0])
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(string(line))
}

// previewReason names a preview's planned features.rea write, judged from
// the planned mappings, or "" when the run would leave autopus.yaml alone.
func (r *reaRun) previewReason(dir string, cfg *config.HarnessConfig) string {
	if !r.active {
		return ""
	}
	next := reaKeyAfterRun(r.state, cfg.Features.Rea.Enabled, reaPlannedManaged(dir, cfg.Platforms, r.action))
	if reaSameKey(cfg.Features.Rea.Enabled, next) {
		return ""
	}
	return fmt.Sprintf("REA decision would be persisted in autopus.yaml (features.rea.enabled: %t)", *next)
}

// finishUpdate runs finish after an update's platform writes and returns its
// failure as one platform error entry.
func (r *reaRun) finishUpdate(out io.Writer, dir string, cfg *config.HarnessConfig) []string {
	if err := r.finish(out, dir, cfg); err != nil {
		return []string{"rea: " + err.Error()}
	}
	return nil
}
