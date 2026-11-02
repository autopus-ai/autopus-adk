package pipeline

import (
	"regexp"
	"strings"
)

// sourceLocation matches a token followed by `:line` or `:line:col`. Whether
// the token is a source location is decided by isSourceLocation, because Go
// regular expressions cannot look behind for a URL scheme.
var sourceLocation = regexp.MustCompile(`([^\s:"'()\[\]<>,;=]+)((?::\d+){1,2})\b`)

// sourceExtensions are the file extensions a source or text location ends
// with. Hostnames end with top-level domains instead, so `host.example.com:443`
// keeps its port.
var sourceExtensions = map[string]bool{
	"go": true, "py": true, "pyi": true, "js": true, "jsx": true, "mjs": true, "cjs": true,
	"ts": true, "tsx": true, "rs": true, "java": true, "kt": true, "kts": true, "scala": true,
	"rb": true, "php": true, "c": true, "h": true, "cc": true, "cpp": true, "cxx": true,
	"hpp": true, "hh": true, "cs": true, "swift": true, "m": true, "mm": true, "sh": true,
	"bash": true, "zsh": true, "ps1": true, "sql": true, "proto": true, "yaml": true,
	"yml": true, "json": true, "toml": true, "ini": true, "md": true, "mdx": true, "txt": true,
	"html": true, "htm": true, "css": true, "scss": true, "sass": true, "less": true,
	"vue": true, "svelte": true, "tf": true, "hcl": true, "ex": true, "exs": true, "erl": true,
	"hs": true, "lua": true, "dart": true, "r": true, "pl": true, "pm": true, "groovy": true,
	"gradle": true, "xml": true, "tmpl": true, "tpl": true, "mod": true, "sum": true,
}

// normalizeFindingText lowercases a finding, collapses its whitespace, and
// masks the line and column of each source location, which move when a fix
// edits the file (SPEC-PIPELINE-RESUME-001 REQ-PRR-FAIL-12). Ports, times,
// and other `:digits` are part of what a finding says and stay.
func normalizeFindingText(text string) string {
	text = strings.ToLower(text)
	var masked strings.Builder
	last := 0
	for _, match := range sourceLocation.FindAllStringSubmatchIndex(text, -1) {
		pathStart, pathEnd := match[2], match[3]
		if !isSourceLocation(text, pathStart, text[pathStart:pathEnd]) {
			continue
		}
		masked.WriteString(text[last:pathEnd])
		masked.WriteString(":#")
		last = match[1]
	}
	masked.WriteString(text[last:])
	return strings.Join(strings.Fields(masked.String()), " ")
}

// isSourceLocation accepts a path that holds a separator or ends with a
// source or text file extension, and rejects the authority of a URL. A colon
// right before the path is a URL scheme unless it closes a drive letter.
func isSourceLocation(text string, start int, path string) bool {
	if start > 0 && text[start-1] == ':' && !isDriveLetter(text, start-1) {
		return false
	}
	if strings.HasPrefix(path, "//") {
		return false
	}
	if strings.ContainsAny(path, `/\`) {
		return true
	}
	dot := strings.LastIndexByte(path, '.')
	return dot > 0 && sourceExtensions[path[dot+1:]]
}

// isDriveLetter reports a single ASCII letter, at the start of a token,
// before the colon at index colon, as in `c:\repo\main.go:12`.
func isDriveLetter(text string, colon int) bool {
	if colon < 1 || text[colon-1] < 'a' || text[colon-1] > 'z' {
		return false
	}
	return colon == 1 || strings.ContainsRune(" \t\n\"'([<", rune(text[colon-2]))
}
