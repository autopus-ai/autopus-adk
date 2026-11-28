package harnessfb

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/filelock"
)

// Paths under the project root. Session records are runtime data; the
// candidates and lessons are the project's own and may be committed.
const (
	SessionsDir   = ".autopus/runtime/harness/sessions"
	RuntimeDir    = ".autopus/runtime/harness"
	CandidatesDir = ".autopus/harness/candidates"
	LessonsDir    = ".autopus/harness/lessons"
	// RuleFile is where approved lessons reach the agent: Claude Code loads
	// every .claude/rules file, and `auto update` neither writes nor prunes
	// this one.
	RuleFile = ".claude/rules/harness-lessons.md"
)

// ErrBusy means another process is reading the same session.
var ErrBusy = errors.New("harnessfb: session busy")

// ProjectRoot is the main work tree of the repository dir is in, so that
// sessions run in linked worktrees count toward one project; dir itself
// outside a repository.
func ProjectRoot(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return dir
	}
	common := strings.TrimSpace(string(out))
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	if top, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(top))
	}
	return dir
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func sessionRel(id string) string {
	return path.Join(SessionsDir, unsafeID.ReplaceAllString(id, "_")+".json")
}

// Observe folds the transcript bytes appended since the last observation
// of session id into its record under root. Only one process reads a
// session at a time; a second gets ErrBusy and the next stop catches up.
func Observe(ctx context.Context, root, id, transcript string) (Session, error) {
	if id == "" || transcript == "" {
		return Session{}, errors.New("harnessfb: session id and transcript are required")
	}
	rel := sessionRel(id)
	if err := prepareDir(root, SessionsDir); err != nil {
		return Session{}, err
	}
	lock, err := filelock.Acquire(ctx, filepath.Join(root, filepath.FromSlash(rel))+".lock", 0)
	if errors.Is(err, filelock.ErrTimeout) {
		return Session{}, ErrBusy
	}
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = lock.Unlock() }()
	s := Session{Metrics: Metrics{SessionID: id}}
	if data, err := readFile(root, rel); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	s.Paths = LoadPathIndex(ctx, root)
	if err := ReadClaudeTranscript(transcript, &s); err != nil {
		return s, err
	}
	return s, writeJSON(root, rel, s)
}

// LoadSessions reads every session record under root that was active
// after since.
func LoadSessions(root string, since time.Time) ([]Metrics, error) {
	names, err := listFiles(root, SessionsDir, ".json")
	if err != nil {
		return nil, err
	}
	var out []Metrics
	for _, name := range names {
		data, err := readFile(root, path.Join(SessionsDir, name))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(data, &s) != nil || s.SessionID == "" || s.End.Before(since) {
			continue
		}
		out = append(out, s.Metrics)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].End.Before(out[j].End) })
	return out, nil
}

// writeJSON writes v at rel inside root.
func writeJSON(root, rel string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(root, rel, append(data, '\n'))
}

// LogRel is where the background run notes what it did.
var LogRel = path.Join(RuntimeDir, "observe.log")

// AppendLog adds one timestamped line to the background run's log.
func AppendLog(root string, now time.Time, msg string) error {
	return appendFile(root, LogRel, []byte(now.UTC().Format(time.RFC3339)+" "+msg+"\n"))
}
