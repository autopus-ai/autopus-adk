package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// statusShellSafe is what a shell word may hold unquoted.
var statusShellSafe = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// shellQuote quotes s as one POSIX shell word: as is when it holds only safe
// characters, otherwise single-quoted with embedded quotes escaped. Values
// from receipts never reach a command unvalidated; directories, which the
// file system names, are always quoted when needed (REQ-SEV-NEXT-04).
func shellQuote(s string) string {
	if statusShellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// commandSafeDir reports a directory that can be shown and quoted on one line.
func commandSafeDir(dir string) bool {
	if dir == "" {
		return false
	}
	for _, r := range dir {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// autoRouteForms spells one /auto route for the given platforms: Codex runs
// its native $codex-auto-<route> skill, the others take /auto <route>.
func autoRouteForms(platforms []string, route, arg string) []statusCommandForm {
	slash, codex := []string{}, []string{}
	for _, platform := range platforms {
		if platform == "codex" {
			codex = append(codex, platform)
		} else {
			slash = append(slash, platform)
		}
	}
	var forms []statusCommandForm
	if len(slash) > 0 || len(codex) == 0 {
		forms = append(forms, statusCommandForm{Platforms: slash, Command: strings.TrimSpace("/auto " + route + " " + arg)})
	}
	if len(codex) > 0 {
		forms = append(forms, statusCommandForm{Platforms: codex, Command: strings.TrimSpace("$codex-auto-" + route + " " + arg)})
	}
	return forms
}

// autoRouteFormsText joins the forms, each followed by its platforms.
func autoRouteFormsText(forms []statusCommandForm) string {
	parts := make([]string, 0, len(forms))
	for _, form := range forms {
		if len(form.Platforms) == 0 {
			parts = append(parts, form.Command)
			continue
		}
		parts = append(parts, form.Command+" ("+statusSingleLine(strings.Join(form.Platforms, ", "))+")")
	}
	return strings.Join(parts, " · ")
}

func renderStatusNext(out io.Writer, actions []statusNextAction) {
	tui.SectionHeader(out, "Next")
	_, _ = fmt.Fprintln(out, "  Nothing below runs by itself.")
	for i, action := range actions {
		_, _ = fmt.Fprintf(out, "  %d. %s\n", i+1, statusActionCommandText(action))
		_, _ = fmt.Fprintf(out, "     %s\n", action.Purpose)
		_, _ = fmt.Fprintf(out, "     %s\n", statusActionFactsText(action))
	}
}

// statusActionCommandText is the line to copy: the shell line of a terminal
// command, which changes into its directory first, or every platform's form
// of an agent-session route.
func statusActionCommandText(action statusNextAction) string {
	if len(action.Forms) > 0 {
		return autoRouteFormsText(action.Forms)
	}
	if action.Shell != "" {
		return action.Shell
	}
	return action.Command
}

func statusActionFactsText(action statusNextAction) string {
	var facts []string
	if action.Optional {
		facts = append(facts, "optional")
	}
	facts = append(facts, "in "+action.Cwd, "runs in "+action.RunsIn, "provider: "+action.Provider,
		"calls: "+action.ExternalCalls, "writes: "+action.Writes)
	return strings.Join(facts, " · ")
}
