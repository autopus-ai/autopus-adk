package cireport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// API reads one GitHub REST path, such as "repos/o/r/actions/runs?page=1".
type API interface {
	Get(ctx context.Context, path string) ([]byte, error)
}

// ErrBudget reports that the request budget of the run is spent.
var ErrBudget = errors.New("ci report: request budget spent")

// GH is the API through the user's own `gh` CLI and its authentication.
// Responses are cached in a directory for the run, so a path is fetched at
// most once, and network requests stop at the budget.
type GH struct {
	bin      string
	cacheDir string
	budget   int
	mu       sync.Mutex
	used     int
}

// NewGH finds gh on PATH. cacheDir must exist and belong to this run.
func NewGH(cacheDir string, budget int) (*GH, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, errors.New("ci report: gh CLI not found on PATH; install it from https://cli.github.com and run `gh auth login`")
	}
	return &GH{bin: bin, cacheDir: cacheDir, budget: budget}, nil
}

// CheckAuth fails clearly when gh has no usable login.
func (g *GH) CheckAuth(ctx context.Context) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, g.bin, "auth", "status")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			detail = ": " + firstLine(detail)
		}
		return fmt.Errorf("ci report: gh is not authenticated (gh auth status failed%s); run `gh auth login` or set GH_TOKEN", detail)
	}
	return nil
}

// Requests is the number of network requests made so far.
func (g *GH) Requests() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.used
}

// Get returns the cached response for path, or fetches it with `gh api`.
func (g *GH) Get(ctx context.Context, path string) ([]byte, error) {
	sum := sha256.Sum256([]byte(path))
	cached := filepath.Join(g.cacheDir, hex.EncodeToString(sum[:])+".json")
	if data, err := os.ReadFile(cached); err == nil {
		return data, nil
	}
	g.mu.Lock()
	if g.used >= g.budget {
		g.mu.Unlock()
		return nil, ErrBudget
	}
	g.used++
	g.mu.Unlock()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, g.bin, "api", "-H", "Accept: application/vnd.github+json", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh api %s: %s", path, firstLine(strings.TrimSpace(stderr.String()+" "+err.Error())))
	}
	_ = os.WriteFile(cached, stdout.Bytes(), 0o600)
	return stdout.Bytes(), nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
