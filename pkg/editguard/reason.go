package editguard

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Reason sizes of REQ-EG-17 and spec.md Decision Output Contract.
const (
	maxReasonBytes  = 1024
	maxDisplayBytes = 256
	displayCut      = maxDisplayBytes - len(displayEllipsis)
	displayEllipsis = "..."
	redacted        = "<redacted>"
)

const (
	gsHead  = "autopus edit-guard [generated_surface]: "
	gsSrc   = "Change the canonical source (content/, templates/, pkg/adapter/) and run: make generate-templates && auto update"
	gsCon   = "Change autopus.yaml or the upstream Autopus source, then run: auto update"
	flUser  = "If the test itself is wrong, stop and ask the user to run: auto fix unlock -- "
	flxUser = "If the test itself is wrong, stop and ask the user to find the path with auto fix lock --list --json and unlock it."
)

// gsReason is GS-SRC in the ADK source repo and GS-CON elsewhere.
func gsReason(display, manifest string, sourceRepo bool) string {
	tail := gsCon
	if sourceRepo {
		tail = gsSrc
	}
	return gsHead + displayPath(display) + " is generated (manifest " + displayPath(manifest) +
		", policy always). " + tail
}

// flReason is FL for the recorded path, or FL-X when the path cannot be
// echoed exactly or the complete FL reason would pass 1024 bytes.
func flReason(recorded string) string {
	head := flHead(recorded)
	if echoable(recorded) {
		if reason := head + flUser + shellQuote(recorded); len(reason) <= maxReasonBytes {
			return reason
		}
	}
	return head + flxUser
}

func flHead(recorded string) string {
	return "autopus edit-guard [fix_lock]: " + displayPath(recorded) +
		" is the locked reproduction test of an in-progress /auto fix. Fix the code under test instead. "
}

func gstReason(rel string) string {
	return "autopus edit-guard [guard_state]: " + displayPath(rel) +
		" is edit-guard state. Use auto fix lock, auto fix unlock, or auto update instead."
}

// echoable reports whether a path survives display unchanged, so the quoted
// argument names the very file the user sees.
func echoable(p string) bool {
	return len(p) <= maxDisplayBytes && utf8.ValidString(p) && !strings.ContainsFunc(p, isControl)
}

// displayPath is the sanitized {path}: control bytes removed, invalid UTF-8
// replaced, an absolute path redacted, and more than 256 bytes cut on a UTF-8
// boundary to at most 253 bytes plus "...".
func displayPath(p string) string {
	clean := strings.ToValidUTF8(strings.Map(dropControl, p), "�")
	if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, `\`) || filepath.IsAbs(clean) {
		return redacted
	}
	if len(clean) <= maxDisplayBytes {
		return clean
	}
	cut := displayCut
	for cut > 0 && !utf8.RuneStart(clean[cut]) {
		cut--
	}
	return clean[:cut] + displayEllipsis
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

func dropControl(r rune) rune {
	if isControl(r) {
		return -1
	}
	return r
}

// shellQuote quotes s for a POSIX shell. Nothing is special inside single
// quotes, so each embedded quote closes the quoting, adds a backslash-escaped
// quote, and reopens it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
