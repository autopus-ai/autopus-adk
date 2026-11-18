// Package adapter의 매니페스트는 autopus-adk가 관리하는 파일 목록과 체크섬을 추적한다.
package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/insajin/autopus-adk/pkg/version"
)

const (
	manifestDir  = ".autopus"
	manifestFile = "manifest.json"
)

// Manifest는 autopus-adk가 관리하는 파일 목록이다.
type Manifest struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	GeneratedAt string `json:"generated_at"`
	// Generator is the auto version that wrote the manifest. A session hook
	// compares it with the running binary to flag a stale installed surface.
	Generator string                  `json:"generator,omitempty"`
	Files     map[string]ManifestFile `json:"files"`
}

// ManifestFile은 매니페스트에 기록된 단일 파일 정보이다.
type ManifestFile struct {
	Checksum string          `json:"checksum"`
	Policy   OverwritePolicy `json:"policy"`
}

// UpdateAction은 업데이트 시 파일별 처리 결과이다.
type UpdateAction string

const (
	ActionOverwrite UpdateAction = "overwrite" // 사용자 미수정 → 덮어쓰기
	ActionBackup    UpdateAction = "backup"    // 사용자 수정 → 백업 후 덮어쓰기
	ActionSkip      UpdateAction = "skip"      // 사용자 삭제 → 스킵
	ActionCreate    UpdateAction = "create"    // 새 파일 → 생성
)

// UpdateResult는 단일 파일의 업데이트 결과이다.
type UpdateResult struct {
	Path       string       `json:"path"`
	Action     UpdateAction `json:"action"`
	BackupPath string       `json:"backup_path,omitempty"`
}

// NewManifest는 빈 매니페스트를 생성한다.
func NewManifest(platform string) *Manifest {
	return &Manifest{
		Version:     "1.0.0",
		Platform:    platform,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Files:       make(map[string]ManifestFile),
	}
}

// ManifestFromFiles는 PlatformFiles로부터 매니페스트를 생성한다.
func ManifestFromFiles(platform string, pf *PlatformFiles) *Manifest {
	m := NewManifest(platform)
	for _, f := range pf.Files {
		m.Files[f.TargetPath] = ManifestFile{
			Checksum: f.Checksum,
			Policy:   f.OverwritePolicy,
		}
	}
	return m
}

// LoadManifest는 프로젝트 루트에서 매니페스트를 로드한다.
// 매니페스트가 없으면 nil을 반환한다 (에러 아님).
func LoadManifest(root, platform string) (*Manifest, error) {
	path := filepath.Join(root, manifestDir, platform+"-"+manifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("매니페스트 읽기 실패: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("매니페스트 파싱 실패: %w", err)
	}
	return &m, nil
}

// Save는 매니페스트를 디스크에 저장한다.
func (m *Manifest) Save(root string) error {
	dir := filepath.Join(root, manifestDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("매니페스트 디렉터리 생성 실패: %w", err)
	}

	m.GeneratedAt = time.Now().Format(time.RFC3339)
	m.Generator = version.Version()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("매니페스트 직렬화 실패: %w", err)
	}

	path := filepath.Join(dir, m.Platform+"-"+manifestFile)
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// ResolveAction은 파일별 업데이트 액션을 결정한다.
//
// 규칙:
//   - 파일 없음 + 매니페스트에 있음 → 사용자가 삭제함 → Skip
//   - 파일 없음 + 매니페스트에 없음 → 새 파일 → Create
//   - 파일 있음 + 체크섬 동일 → 사용자 미수정 → Overwrite
//   - 파일 있음 + 체크섬 다름 → 사용자 수정함 → Backup
//   - 마커 정책 파일 → 항상 Overwrite (마커 섹션만 교체하므로 안전)
func ResolveAction(root string, targetPath string, policy OverwritePolicy, old *Manifest) UpdateAction {
	// 마커 정책은 항상 안전하게 덮어쓰기
	if policy == OverwriteMarker {
		return ActionOverwrite
	}

	filePath := filepath.Join(root, targetPath)
	_, err := os.Stat(filePath)
	fileExists := err == nil

	// 이전 매니페스트가 없으면 init 직후 → Create 또는 Overwrite
	if old == nil {
		if fileExists {
			return ActionOverwrite
		}
		return ActionCreate
	}

	prevFile, wasManaged := old.Files[targetPath]

	if !fileExists {
		if wasManaged {
			return ActionSkip
		}
		return ActionCreate
	}

	if wasManaged {
		currentData, err := os.ReadFile(filePath)
		if err != nil {
			return ActionBackup
		}
		if Checksum(string(currentData)) == prevFile.Checksum {
			return ActionOverwrite
		}
		return ActionBackup
	}

	return ActionBackup
}

// BackupFile은 파일을 백업 디렉터리로 복사한다.
func BackupFile(root, targetPath, backupDir string) (string, error) {
	srcPath := filepath.Join(root, targetPath)
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("백업 원본 읽기 실패 %s: %w", targetPath, err)
	}

	backupPath := filepath.Join(backupDir, targetPath)
	backupParent := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupParent, 0755); err != nil {
		return "", fmt.Errorf("백업 디렉터리 생성 실패: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("백업 파일 쓰기 실패: %w", err)
	}

	return backupPath, nil
}

// CreateBackupDir은 타임스탬프 기반 백업 디렉터리를 생성한다.
func CreateBackupDir(root string) (string, error) {
	ts := time.Now().Format("20060102T150405")
	dir := filepath.Join(root, manifestDir, "backup", ts)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("백업 디렉터리 생성 실패: %w", err)
	}
	return dir, nil
}

// Checksum은 문자열의 SHA256 체크섬을 반환한다.
func Checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
