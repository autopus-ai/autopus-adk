package opencode

import (
	"encoding/json"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/editguard"
)

// openCodeGuardTools names the native file-editing tools whose calls a
// generated plugin sends through `auto guard edit` (REQ-EG-13), and which of
// them carry a patch.
type openCodeGuardTools struct {
	tools, patchTools []string
}

var (
	// openCodeGuardToolsV2 are the tools OpenCode 2.0.10 offered in probe A2:
	// edit and write with key path and, for GPT-family model keys, patch with
	// patchText (testdata/opencode-2.0.10-execute-before.jsonl).
	openCodeGuardToolsV2 = openCodeGuardTools{tools: []string{"edit", "write", "patch"}, patchTools: []string{"patch"}}
	// openCodeGuardToolsV1 is host-unverified: probe A2 has no OpenCode 1.x run
	// (CD-1 open). It adds to the 2.0.10 names the V1 names the 2.0.10
	// compatibility layer still maps (apply_patch to patch) and multiedit; the
	// plugin reads filePath, the V1 argument key, beside path.
	openCodeGuardToolsV1 = openCodeGuardTools{
		tools:      []string{"edit", "write", "multiedit", "patch", "apply_patch"},
		patchTools: []string{"patch", "apply_patch"},
	}
)

// guardPluginSpec is the EDIT_GUARD literal of a generated plugin: the argv
// the plugin spawns without a shell, its timeout in seconds, and its tools.
type guardPluginSpec struct {
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Timeout    int      `json:"timeout"`
	Tools      []string `json:"tools"`
	PatchTools []string `json:"patchTools"`
}

// splitEditGuard separates the edit-guard registration pkg/content generates
// for OpenCode from the shell-tool hooks and renders it as the plugin's
// EDIT_GUARD literal, which is null without a registration.
func splitEditGuard(hooks []adapter.HookConfig, tools openCodeGuardTools) ([]adapter.HookConfig, string, error) {
	command := content.EditGuardCommand(editguard.PlatformOpenCode)
	rest := make([]adapter.HookConfig, 0, len(hooks))
	var guard *guardPluginSpec
	for _, hook := range hooks {
		if hook.Command != command || !strings.EqualFold(hook.Event, "PreToolUse") {
			rest = append(rest, hook)
			continue
		}
		argv := strings.Fields(hook.Command)
		guard = &guardPluginSpec{Command: argv[0], Args: argv[1:], Timeout: hook.Timeout,
			Tools: tools.tools, PatchTools: tools.patchTools}
	}
	if guard == nil {
		return rest, "null", nil
	}
	encoded, err := json.Marshal(guard)
	return rest, string(encoded), err
}

// openCodeGuardRuntime is the plugin code shared by the V1 and V2 plugins. It
// synthesizes the payload of the Decision Output Contract and throws only for
// a deny decision a guard printed before exiting 0; a spawn failure, a
// timeout, oversized output, or any other exit resolves (REQ-EG-13).
const openCodeGuardRuntime = `const GUARD_OUTPUT_LIMIT = 65536
const PATCH_HEADERS = ["*** Add File: ", "*** Delete File: ", "*** Update File: "]

// patchTargets reads the paths of an OpenCode patch in patch order, move
// destinations included, at the header positions the OpenCode 2.0.10 patch
// parser reads. Where the host would reject a patch, extra paths are harmless.
function patchTargets(text) {
  if (typeof text !== "string") return []
  const trimmed = text.trim()
  const body = trimmed.match(/^(?:cat\s+)?<<(['"]?)(\w+)\1\s*\n([\s\S]*?)\n\2\s*$/)?.[3] ?? trimmed
  const lines = body.split("\n").map((line) => (line.endsWith("\r") ? line.slice(0, -1) : line))
  const isHeader = (line) => line === "*** End Patch" || PATCH_HEADERS.some((marker) => line.startsWith(marker))
  const targets = []
  let hunk = ""
  for (let i = 0; i < lines.length; i++) {
    const header = lines[i].trim()
    if (hunk === "add" && !isHeader(header)) continue
    if (hunk === "update" && !isHeader(lines[i].trimEnd())) continue
    hunk = ""
    if (header.startsWith(PATCH_HEADERS[0])) {
      targets.push(header.slice(PATCH_HEADERS[0].length).trim())
      hunk = "add"
    } else if (header.startsWith(PATCH_HEADERS[1])) {
      targets.push(header.slice(PATCH_HEADERS[1].length).trim())
    } else if (header.startsWith(PATCH_HEADERS[2])) {
      targets.push(header.slice(PATCH_HEADERS[2].length).trim())
      let next = i + 1
      while (lines[next]?.trimEnd() === "*** End of File") next++
      const move = lines[next]?.trimEnd()
      if (move === "*** Move to:" || move?.startsWith("*** Move to: ")) {
        targets.push(move.slice(13).trim())
        i = next
      }
      hunk = "update"
    }
  }
  return targets
}

// guardTargets lists every path a file-editing call writes, in native
// argument order; a non-string or empty path is no target.
function guardTargets(tool, args) {
  if (!args || typeof args !== "object") return []
  const targets = EDIT_GUARD.patchTools.includes(tool) ? patchTargets(args.patchText) : []
  for (const key of ["path", "filePath", "filepath"]) {
    if (typeof args[key] === "string") targets.push(args[key])
  }
  if (Array.isArray(args.edits)) {
    for (const edit of args.edits) if (typeof edit?.filePath === "string") targets.push(edit.filePath)
  }
  return targets.filter((target) => target !== "")
}

function denyReason(stdout) {
  try {
    const decision = JSON.parse(stdout)
    if (decision?.decision === "deny" && typeof decision.reason === "string" && decision.reason) return decision.reason
  } catch {}
  return undefined
}

// runEditGuard returns the deny reason of a guard that exited 0, and
// undefined for every other outcome, so a guard fault never blocks an edit.
function runEditGuard(payload, cwd, running) {
  return new Promise((resolve) => {
    let child
    try {
      child = spawn(EDIT_GUARD.command, EDIT_GUARD.args, {
        cwd, env: process.env, detached: process.platform !== "win32", stdio: ["pipe", "pipe", "ignore"],
      })
    } catch {
      resolve(undefined)
      return
    }
    let stdout = ""
    let settled = false
    const finish = (reason) => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      running?.delete(child)
      child.stdout?.destroy()
      resolve(reason)
    }
    const stop = () => {
      try {
        if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL")
        else child.kill("SIGKILL")
      } catch {}
      finish(undefined)
    }
    const seconds = Number.isFinite(EDIT_GUARD.timeout) && EDIT_GUARD.timeout > 0 ? Math.min(EDIT_GUARD.timeout, 300) : 5
    const timer = setTimeout(stop, seconds * 1000)
    running?.set(child, stop)
    child.on("error", () => finish(undefined))
    child.stdin.on("error", () => {})
    child.stdout.setEncoding("utf8")
    child.stdout.on("data", (chunk) => {
      stdout += chunk
      if (stdout.length > GUARD_OUTPUT_LIMIT) stop()
    })
    child.on("close", (code, signal) => finish(code === 0 && signal === null ? denyReason(stdout) : undefined))
    child.stdin.end(JSON.stringify(payload))
  })
}

async function guardEdit(tool, args, cwd, running) {
  const payload = { platform: "opencode", cwd, tool_name: tool, targets: guardTargets(tool, args) }
  const reason = await runEditGuard(payload, cwd, running)
  if (reason !== undefined) throw new Error(reason)
}
`
