package harnessfb

import (
	"path"
	"regexp"
	"strings"
)

// toolInput is the part of a tool call's input classify reads.
type toolInput struct {
	Command      string `json:"command"`
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`
	To           string `json:"to"`
}

// testCommand matches a shell command that runs a test suite.
var testCommand = regexp.MustCompile(`(^|[\s;&|(])(go test|gotestsum|pytest|python3? -m (pytest|unittest)|` +
	`(npm|pnpm|yarn|bun)( run)? test|npx (vitest|jest)|vitest|jest|cargo (test|nextest)|mvn( -\S+)* (test|verify)|` +
	`gradlew? (test|check)|make (test|check)|auto (qa (run|exec)|test))\b`)

// reviewAgent matches a sub-agent type that reviews, reviewTask a
// description that asks for a review in its first words, and fixTask one
// that starts by asking to fix what a review found.
var (
	reviewAgent = regexp.MustCompile(`(?i)review|audit`)
	reviewTask  = regexp.MustCompile(`(?i)^\W*(\S+\s+){0,2}(re-?)?review\b|^\W*(\S+\s+){0,2}\S*리뷰`)
	fixTask     = regexp.MustCompile(`(?i)^\W*(fix|apply|address|implement|resolve|repair|handle|rework|correct|patch|update)\b|수정|반영|고치|고쳐`)
)

// classify names what a tool call spends time on: "test" for a shell
// command that runs tests, "review" for a reviewer sub-agent (or a message
// that resumes one), "agent" for another sub-agent (it may edit files),
// "bash" for any other shell command (its result may be a timeout).
func classify(tool string, in toolInput, reviewers map[string]bool) string {
	switch tool {
	case "Bash":
		if testCommand.MatchString(in.Command) {
			return "test"
		}
		return "bash"
	case "Agent", "Task":
		if !fixTask.MatchString(in.Description) && (reviewTask.MatchString(in.Description) || reviewAgent.MatchString(in.SubagentType)) {
			return "review"
		}
		return "agent"
	case "SendMessage":
		if reviewers[in.To] {
			return "review"
		}
	}
	return ""
}

// editCommand matches a shell command that edits files in place.
var editCommand = regexp.MustCompile(`\bsed -i|\bperl -\w*i|apply_patch|\bgit (apply|am|restore|checkout --)|` +
	`cat >|\btee\b|python3? - <<|>\s*[\w./-]+\.\w+\b`)

// isEdit reports a tool call that changes files.
func isEdit(tool, command string) bool {
	switch tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return true
	case "Bash":
		return editCommand.MatchString(command)
	}
	return false
}

// notPrompt are the user-role texts the harness writes, not the user.
var notPrompt = []string{"Stop hook feedback", "<task-notification", "[SYSTEM", "<system-reminder",
	"Caveat:", "<command-", "<local-command", "[Request interrupted", "<agent-message", "This session is being continued",
	"Another Claude session sent a message", "[Cross-session delivery notice]", "Base directory for this skill", "<teammate-message"}

// isPrompt reports a user-role text the user typed.
func isPrompt(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	for _, p := range notPrompt {
		if strings.HasPrefix(t, p) {
			return false
		}
	}
	return true
}

var notificationID = regexp.MustCompile(`<tool-use-id>([^<\s]+)</tool-use-id>`)

// notification returns the tool call a background-task notification ends.
func notification(text string) (string, bool) {
	if !strings.Contains(text, "<task-notification>") {
		return "", false
	}
	m := notificationID.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// blockKind names what a Stop hook block was about, from the hook's
// command and, for the verification gate, how its reason starts.
func blockKind(command, reason string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "other"
	}
	// "auto verify stop --budget 15m0s" is still the verification gate.
	verb := ""
	if len(fields) >= 3 && path.Base(fields[0]) == "auto" && fields[1] == "verify" {
		verb = fields[2]
	}
	switch verb {
	case "language":
		return "language"
	case "stop":
		switch {
		case strings.HasPrefix(reason, "Verification failed"):
			return "verify"
		case strings.HasPrefix(reason, "Possible credential"):
			return "secret"
		case strings.HasPrefix(reason, "Test-first"):
			return "test-first"
		case strings.Contains(reason, "verification round"):
			return "followup"
		case strings.Contains(reason, "versions"):
			return "version"
		}
		return "verify-other"
	}
	return "hook:" + path.Base(fields[0])
}
