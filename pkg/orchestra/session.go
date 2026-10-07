package orchestra

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

const sessionDirectoryName = "autopus-orchestra-sessions"

type sessionWritableFile interface {
	Stat() (fs.FileInfo, error)
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// OrchestraSession holds state for a yield-rounds orchestra session.
type OrchestraSession struct {
	ID            string                      `json:"id"`
	TerminalKind  string                      `json:"terminal_kind,omitempty"`
	WorkspaceRef  string                      `json:"workspace_ref,omitempty"`
	TmuxServerRef string                      `json:"tmux_server_ref,omitempty"`
	Panes         map[string]string           `json:"panes"` // provider name -> pane ID
	Providers     []SessionProviderConfig     `json:"providers"`
	Rounds        [][]SessionProviderResponse `json:"rounds"`
	CreatedAt     time.Time                   `json:"created_at"`
}

// SessionProviderConfig is a serializable subset of ProviderConfig.
type SessionProviderConfig struct {
	Name   string `json:"name"`
	Binary string `json:"binary"`
}

// SessionProviderResponse is a serializable subset of ProviderResponse.
type SessionProviderResponse struct {
	Provider        string                    `json:"provider"`
	Output          string                    `json:"output"`
	DurationMs      int64                     `json:"duration_ms"`
	TimedOut        bool                      `json:"timed_out"`
	Usage           []telemetry.UsageEnvelope `json:"usage,omitempty"`
	UsageCapability UsageCapability           `json:"usage_capability"`
}

func sessionDirectoryPath() string {
	return filepath.Join(os.TempDir(), sessionDirectoryName)
}

func sessionFilePath(id string) string {
	if validateSafeArtifactName("session ID", id) != nil {
		return ""
	}
	return filepath.Join(sessionDirectoryPath(), id+".json")
}

func openSessionRoot(create bool) (*os.Root, error) {
	tempRoot, err := os.OpenRoot(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("open temp root: %w", err)
	}
	defer func() { _ = tempRoot.Close() }()

	if create {
		if err := tempRoot.Mkdir(sessionDirectoryName, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create session directory: %w", err)
		}
	}
	info, err := tempRoot.Lstat(sessionDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("inspect session directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("session directory is not a private directory")
	}
	root, err := tempRoot.OpenRoot(sessionDirectoryName)
	if err != nil {
		return nil, fmt.Errorf("open session directory: %w", err)
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("inspect opened session directory: %w", err)
	}
	if !os.SameFile(info, rootInfo) {
		_ = root.Close()
		return nil, fmt.Errorf("session directory changed while opening")
	}
	if runtime.GOOS != "windows" && rootInfo.Mode().Perm() != 0o700 {
		_ = root.Close()
		return nil, fmt.Errorf("session directory permissions are %o, want 700", rootInfo.Mode().Perm())
	}
	return root, nil
}

func sessionFilename(id string) (string, error) {
	if err := validateSafeArtifactName("session ID", id); err != nil {
		return "", fmt.Errorf("invalid session ID: %w", err)
	}
	return id + ".json", nil
}

// SaveSession atomically claims a private session file and never overwrites it.
func SaveSession(session OrchestraSession) error {
	name, err := sessionFilename(session.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	root, err := openSessionRoot(true)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create session %s: %w", session.ID, err)
	}
	return persistCreatedSession(root, name, file, data)
}

func persistCreatedSession(root *os.Root, name string, file sessionWritableFile, data []byte) error {
	createdInfo, statErr := file.Stat()
	rollback := func(cause error) error {
		closeErr := file.Close()
		rollbackCreatedSession(root, name, createdInfo)
		if closeErr != nil {
			return errors.Join(cause, fmt.Errorf("close session: %w", closeErr))
		}
		return cause
	}
	if statErr != nil {
		return rollback(fmt.Errorf("inspect created session: %w", statErr))
	}
	if err := file.Chmod(0o600); err != nil {
		return rollback(fmt.Errorf("set session permissions: %w", err))
	}
	written, err := file.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return rollback(fmt.Errorf("write session: %w", err))
	}
	if err := file.Sync(); err != nil {
		return rollback(fmt.Errorf("sync session: %w", err))
	}
	if err := file.Close(); err != nil {
		rollbackCreatedSession(root, name, createdInfo)
		return fmt.Errorf("close session: %w", err)
	}
	return nil
}

func rollbackCreatedSession(root *os.Root, name string, created fs.FileInfo) {
	if created == nil {
		return
	}
	current, err := root.Lstat(name)
	if err == nil && current.Mode().IsRegular() && os.SameFile(created, current) {
		_ = root.Remove(name)
	}
}

type sessionCommitFunc func(root *os.Root, temporary, target string) error

// UpdateSession atomically replaces an existing private or legacy session. It
// never removes the target before the fully synced replacement is ready, so a
// failed update leaves the previous cleanup handle available for retry.
func UpdateSession(session OrchestraSession) error {
	return updateSessionWithCommit(session, func(root *os.Root, temporary, target string) error {
		return root.Rename(temporary, target)
	})
}

func updateSessionWithCommit(session OrchestraSession, commit sessionCommitFunc) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session update: %w", err)
	}
	root, name, originalInfo, err := openSessionUpdateTarget(session.ID)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	temporary := name + "." + NewSessionID() + ".tmp"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create session update: %w", err)
	}
	if err := persistCreatedSession(root, temporary, file, data); err != nil {
		return fmt.Errorf("persist session update: %w", err)
	}
	defer func() { _ = root.Remove(temporary) }()
	currentInfo, err := root.Lstat(name)
	if err != nil || !currentInfo.Mode().IsRegular() || !os.SameFile(originalInfo, currentInfo) {
		return errors.New("session entry changed before update")
	}
	if commit == nil {
		return errors.New("nil session update commit")
	}
	if err := commit(root, temporary, name); err != nil {
		return fmt.Errorf("commit session update: %w", err)
	}
	return nil
}
