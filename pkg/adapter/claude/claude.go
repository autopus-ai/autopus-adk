// Package claude는 Claude Code 플랫폼 어댑터를 구현한다.
package claude

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"

	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

const (
	markerBegin = "<!-- AUTOPUS:BEGIN -->"
	markerEnd   = "<!-- AUTOPUS:END -->"
	adapterName = "claude-code"
	cliBinary   = "claude"
	adapterVer  = "1.0.0"
)

// Adapter는 Claude Code 플랫폼 어댑터이다.
// @AX:ANCHOR: PlatformAdapter 인터페이스의 claude-code 구현체 — 다수의 CLI 커맨드에서 사용됨
// @AX:REASON: [AUTO] init/update/doctor/platform 커맨드에서 참조
type Adapter struct {
	root           string                // project root path
	engine         *tmpl.Engine          // template rendering engine
	statusLineMode config.StatusLineMode // runtime-only statusline override
	// suppressCommitAttribution mirrors lore.forbidden_trailers for this run.
	suppressCommitAttribution bool
}

// New는 현재 디렉터리를 루트로 하는 어댑터를 생성한다.
func New() *Adapter {
	return &Adapter{
		root:   ".",
		engine: tmpl.New(),
	}
}

// NewWithRoot는 지정된 루트 경로로 어댑터를 생성한다.
func NewWithRoot(root string) *Adapter {
	return &Adapter{
		root:   root,
		engine: tmpl.New(),
	}
}

func (a *Adapter) Name() string        { return adapterName }
func (a *Adapter) Version() string     { return adapterVer }
func (a *Adapter) CLIBinary() string   { return cliBinary }
func (a *Adapter) SupportsHooks() bool { return true }

// Detect는 PATH에서 claude 바이너리 설치 여부를 확인한다.
func (a *Adapter) Detect(_ context.Context) (bool, error) {
	_, err := exec.LookPath(cliBinary)
	return err == nil, nil
}

// checksum은 문자열의 SHA256 체크섬을 반환한다.
func checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
