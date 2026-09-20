package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillAuditFileLimit = 1 << 20
const skillAuditEntryLimit = 4096

type skillAuditFile struct {
	MetadataTokenEstimate int    `json:"metadata_token_estimate"`
	BodyTokenEstimate     int    `json:"body_token_estimate"`
	Path                  string `json:"path"`
	Name                  string `json:"name"`
	SHA256                string `json:"sha256"`
	MetadataBytes         int    `json:"metadata_bytes"`
	BodyBytes             int    `json:"body_bytes"`
}

func skillAuditRoots(platform string) ([]string, error) {
	switch platform {
	case "codex":
		return []string{".codex/skills", ".agents/skills", ".autopus/plugins/auto/skills"}, nil
	case "opencode":
		return []string{".agents/skills", ".opencode/skills"}, nil
	case "claude", "claude-code":
		return []string{".claude/skills"}, nil
	case "gemini", "gemini-cli", "antigravity-cli":
		return []string{".gemini/skills/autopus"}, nil
	case "omp":
		return []string{".omp/skills", ".agents/skills"}, nil
	default:
		return nil, fmt.Errorf("unsupported platform %q", platform)
	}
}

// safeSkillAuditPath rejects symlinks at every managed relative component.
func safeSkillAuditPath(root, rel string) (os.FileInfo, error) {
	path := root
	var info os.FileInfo
	for _, part := range strings.Split(filepath.FromSlash(rel), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		var err error
		info, err = os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink: %s", rel)
		}
	}
	return info, nil
}

func scanSkillAuditFiles(root string, roots []string) ([]skillAuditFile, []string, error) {
	sandbox, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = sandbox.Close() }()
	files := []skillAuditFile{}
	skipped := []string{}
	count := 0
	for _, rel := range roots {
		info, err := safeSkillAuditPath(root, rel)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() {
			skipped = append(skipped, rel)
			continue
		}
		dir, err := sandbox.Open(filepath.FromSlash(rel))
		if err != nil {
			return nil, nil, err
		}
		entries, readErr := dir.ReadDir(skillAuditEntryLimit + 1)
		closeErr := dir.Close()
		if readErr != nil && readErr != io.EOF {
			return nil, nil, readErr
		}
		if closeErr != nil {
			return nil, nil, closeErr
		}
		if len(entries) > skillAuditEntryLimit {
			return nil, nil, fmt.Errorf("skill audit entry limit exceeded: %s", rel)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			count++
			if count > skillAuditEntryLimit {
				return nil, nil, fmt.Errorf("skill audit entry limit exceeded")
			}
			path := filepath.ToSlash(filepath.Join(rel, entry.Name(), "SKILL.md"))
			if !entry.IsDir() {
				if entry.Type()&os.ModeSymlink != 0 {
					skipped = append(skipped, filepath.ToSlash(filepath.Join(rel, entry.Name())))
				}
				continue
			}
			info, err := safeSkillAuditPath(root, path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil || !info.Mode().IsRegular() || info.Size() > skillAuditFileLimit {
				skipped = append(skipped, path)
				continue
			}
			f, err := sandbox.Open(filepath.FromSlash(path))
			if err != nil {
				return nil, nil, err
			}
			data, readErr := io.ReadAll(io.LimitReader(f, skillAuditFileLimit+1))
			closeErr := f.Close()
			if readErr != nil {
				return nil, nil, readErr
			}
			if closeErr != nil {
				return nil, nil, closeErr
			}
			if len(data) > skillAuditFileLimit {
				skipped = append(skipped, path)
				continue
			}
			metadata, body, name, err := skillAuditParts(data)
			if err != nil {
				skipped = append(skipped, path+" (invalid metadata)")
				continue
			}
			files = append(files, skillAuditFile{Path: path, Name: name, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), MetadataBytes: len(metadata), BodyBytes: len(body), MetadataTokenEstimate: skillAuditTokenEstimate(len(metadata)), BodyTokenEstimate: skillAuditTokenEstimate(len(body))})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	sort.Strings(skipped)
	return files, skipped, nil
}

func skillAuditParts(data []byte) ([]byte, []byte, string, error) {
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) == 0 || string(bytes.TrimSpace(lines[0])) != "---" {
		return nil, data, "", nil
	}
	start := len(lines[0])
	offset := start
	for _, line := range lines[1:] {
		if string(bytes.TrimSpace(line)) == "---" {
			metadata := data[start:offset]
			var header struct {
				Name string `yaml:"name"`
			}
			if err := yaml.Unmarshal(metadata, &header); err != nil {
				return nil, nil, "", err
			}
			return metadata, data[offset+len(line):], header.Name, nil
		}
		offset += len(line)
	}
	return nil, nil, "", fmt.Errorf("unclosed frontmatter")
}

func skillAuditGroups(files []skillAuditFile, byName bool) [][]string {
	groups := map[string][]string{}
	for _, file := range files {
		key := file.SHA256
		if byName {
			key = file.Name
		}
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], file.Path)
	}
	result := [][]string{}
	for _, paths := range groups {
		if len(paths) > 1 {
			sort.Strings(paths)
			result = append(result, paths)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i][0] < result[j][0] })
	return result
}
