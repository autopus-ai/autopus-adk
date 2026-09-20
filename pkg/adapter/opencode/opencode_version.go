package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// Option customizes runtime-specific generation without changing project config.
type Option func(*Adapter)

// WithCLIVersion pins the version response for reproducible offline generation.
// Empty or unreadable versions retain the legacy V1 generation contract.
func WithCLIVersion(version string) Option {
	return func(a *Adapter) { a.pinnedVersion = &version }
}

var openCodeVersionPattern = regexp.MustCompile(`\bv?([0-9]+)\.[0-9]+\.[0-9]+\b`)

func (a *Adapter) runtimeMajor() int {
	a.versionOnce.Do(func() {
		version := ""
		if a.pinnedVersion != nil {
			version = *a.pinnedVersion
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			output, err := processprobe.OutputLimited(exec.CommandContext(ctx, cliBinary, "--version"), 8192)
			if err == nil {
				version = string(output)
			}
		}
		a.major = 1
		if match := openCodeVersionPattern.FindStringSubmatch(version); len(match) == 2 {
			if major, err := strconv.Atoi(match[1]); err == nil {
				a.major = major
				a.versionKnown = true
			}
		}
	})
	return a.major
}

func (a *Adapter) isV2() bool { return a.runtimeMajor() == 2 }

func (a *Adapter) validateRuntime() error {
	major := a.runtimeMajor()
	if !a.versionKnown {
		body, err := os.ReadFile(filepath.Join(a.root, ".opencode", "plugins", "autopus-hooks.js"))
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("existing OpenCode plugin unreadable: %w", err)
		}
		if strings.Contains(string(body), "Plugin.define") || strings.Contains(string(body), "// Autopus OpenCode V2 native plugin") {
			return fmt.Errorf("unknown OpenCode runtime: cannot replace existing V2 plugin")
		}
	}
	if major != 1 && major != 2 {
		return fmt.Errorf("unsupported OpenCode major version: %d", major)
	}
	return nil
}
