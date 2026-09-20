package opencode

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
)

func (a *Adapter) preparePluginMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	hooks, _, err := content.GenerateProjectHookConfigs(cfg, adapterName, true)
	if err != nil {
		return nil, fmt.Errorf("hook 생성 실패: %w", err)
	}
	mapping, err := a.prepareHookPluginMapping(hooks)
	if err != nil {
		return nil, err
	}
	return []adapter.FileMapping{mapping}, nil
}

func (a *Adapter) prepareHookPluginMapping(hooks []adapter.HookConfig) (adapter.FileMapping, error) {
	plugin, err := renderHookPlugin(hooks)
	if a.isV2() {
		plugin, err = renderHookPluginV2(hooks)
	}
	if err != nil {
		return adapter.FileMapping{}, err
	}
	return adapter.FileMapping{
		TargetPath:      filepath.Join(".opencode", "plugins", "autopus-hooks.js"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        adapter.Checksum(plugin),
		Content:         []byte(plugin),
	}, nil
}

func (a *Adapter) prepareGitHookMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	_, gitHooks, err := content.GenerateProjectHookConfigs(cfg, adapterName, false)
	if err != nil {
		return nil, fmt.Errorf("git hook 생성 실패: %w", err)
	}
	files := make([]adapter.FileMapping, 0, len(gitHooks))
	for _, hook := range gitHooks {
		files = append(files, adapter.FileMapping{
			TargetPath:      hook.Path,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(hook.Content),
			Content:         []byte(hook.Content),
		})
	}
	return adapter.FilterUnsupportedRootGitHookFiles(a.root, files), nil
}

func renderHookPlugin(hooks []adapter.HookConfig) (string, error) {
	var before []string
	var after []string
	for _, hook := range hooks {
		entry := fmt.Sprintf(`  { command: %q, timeout: %d }`, hook.Command, hook.Timeout)
		switch strings.ToLower(hook.Event) {
		case strings.ToLower("PreToolUse"):
			before = append(before, entry)
		case strings.ToLower("PostToolUse"):
			after = append(after, entry)
		}
	}

	plugin := fmt.Sprintf(`import { spawn } from "node:child_process"

const BEFORE_HOOKS = [
%s
]

const AFTER_HOOKS = [
%s
]

function runCommand(command, cwd, timeoutSeconds) {
  return new Promise((resolve, reject) => {
    const child = spawn("sh", ["-lc", command], {
      cwd,
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    })

    let stderr = ""
    let stdout = ""
    child.stdout.on("data", (chunk) => {
      stdout += chunk.toString()
    })
    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString()
    })

    const timer = setTimeout(() => {
      child.kill("SIGTERM")
      reject(new Error("Autopus hook timed out: " + command))
    }, timeoutSeconds * 1000)

    child.on("close", (code) => {
      clearTimeout(timer)
      if (code === 0) {
        resolve()
        return
      }
      const details = (stderr || stdout).trim()
      reject(new Error(details ? details : "Autopus hook failed: " + command))
    })
  })
}

async function runHooks(hooks, cwd) {
  for (const hook of hooks) {
    await runCommand(hook.command, cwd, hook.timeout)
  }
}

export const AutopusHooksPlugin = async ({ directory, worktree }) => {
  const cwd = worktree || directory
  return {
    "tool.execute.before": async (input) => {
      if (input.tool !== "bash") return
      await runHooks(BEFORE_HOOKS, cwd)
    },
    "tool.execute.after": async (input) => {
      if (input.tool !== "bash") return
      await runHooks(AFTER_HOOKS, cwd)
    },
  }
}

export default AutopusHooksPlugin
`, strings.Join(before, ",\n"), strings.Join(after, ",\n"))
	return plugin, nil
}
