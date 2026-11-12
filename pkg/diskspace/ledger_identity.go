package diskspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ledgerTokenFile is the file, in a linked worktree's administrative
// directory ($GIT_DIR/worktrees/<id>), that holds the identity token the
// ledger recorded at registration. Git deletes that directory whenever the
// worktree is removed or pruned, so a worktree created later at the same
// path never carries the token, even when the filesystem hands the new
// directory the freed inode number (ext4 does).
const ledgerTokenFile = "autopus-disk-token"

// ledgerTokenLen is the length of a token: 16 random bytes in hex.
const ledgerTokenLen = 32

// linkedAdminDir returns the administrative directory of the linked
// worktree at path. Its .git must be a regular gitdir file naming an
// existing directory; anything else is an error.
func linkedAdminDir(path string) (string, error) {
	dotGit := filepath.Join(path, ".git")
	if info, err := os.Lstat(dotGit); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a gitdir file", dotGit)
	}
	dir := worktreeAdminDir(path)
	if dir == "" {
		return "", fmt.Errorf("%s has no gitdir line", dotGit)
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	return dir, nil
}

// readWorktreeToken returns the token in the worktree's administrative
// directory, or "" when there is none or it is malformed.
func readWorktreeToken(path string) string {
	admin, err := linkedAdminDir(path)
	if err != nil {
		return ""
	}
	file := filepath.Join(admin, ledgerTokenFile)
	if info, err := os.Lstat(file); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	token := strings.TrimSpace(string(data))
	if !validToken(token) {
		return ""
	}
	return token
}

// ensureWorktreeToken returns the worktree's token, creating it when the
// administrative directory holds none. Creation links a fully written
// temporary file into place, so concurrent registrations agree on one token
// and nobody reads a partial one.
func ensureWorktreeToken(path string) (string, error) {
	if token := readWorktreeToken(path); token != "" {
		return token, nil
	}
	admin, err := linkedAdminDir(path)
	if err != nil {
		return "", fmt.Errorf("%s %w: %v", path, ErrNotLinkedWorktree, err)
	}
	raw := make([]byte, ledgerTokenLen/2)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(admin, ledgerTokenFile+".tmp-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = tmp.WriteString(hex.EncodeToString(raw) + "\n")
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Link(tmp.Name(), filepath.Join(admin, ledgerTokenFile)); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	if token := readWorktreeToken(path); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("%s: the worktree identity token is unreadable", path)
}

// worktreeHead returns the trimmed HEAD file of the worktree's
// administrative directory: "ref: refs/heads/<branch>" on a branch, the
// commit id when detached, or "" when unreadable.
func worktreeHead(path string) string {
	admin, err := linkedAdminDir(path)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(admin, "HEAD"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func validToken(token string) bool {
	if len(token) != ledgerTokenLen {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}
