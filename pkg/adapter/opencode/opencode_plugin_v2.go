package opencode

import (
	"encoding/json"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func renderHookPluginV2(hooks []adapter.HookConfig) (string, error) {
	type hookCommand struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	before, after := []hookCommand{}, []hookCommand{}
	for _, hook := range hooks {
		command := hookCommand{hook.Command, hook.Timeout}
		switch strings.ToLower(hook.Event) {
		case "pretooluse":
			before = append(before, command)
		case "posttooluse":
			after = append(after, command)
		}
	}
	encodedBefore, err := json.Marshal(before)
	if err != nil {
		return "", err
	}
	encodedAfter, err := json.Marshal(after)
	if err != nil {
		return "", err
	}
	return strings.NewReplacer("__BEFORE_HOOKS__", string(encodedBefore), "__AFTER_HOOKS__", string(encodedAfter)).Replace(openCodePluginV2Source), nil
}

// Hook signatures and session.location come from @opencode/plugin 2.0.10.
// Model input cannot supply a hook command; only generated project config can.
const openCodePluginV2Source = `// Autopus OpenCode V2 native plugin
import { spawn } from "node:child_process"
import path from "node:path"

const BEFORE_HOOKS = __BEFORE_HOOKS__
const AFTER_HOOKS = __AFTER_HOOKS__
const OUTPUT_LIMIT = 65536

function bounded(promise, milliseconds) {
  let timer
  return Promise.race([
    promise,
    new Promise((_, reject) => { timer = setTimeout(() => reject(new Error("Autopus session lookup timed out")), milliseconds) }),
  ]).finally(() => clearTimeout(timer))
}

export default {
  id: "autopus.hooks",
  async setup(ctx) {
    const registrations = []
    const contexts = new Map()
    const running = new Map()
    let disposed = false

    function runCommand(hook, cwd) {
      if (disposed) return Promise.reject(new Error("Autopus hook plugin is disposed"))
      return new Promise((resolve, reject) => {
        const timeout = Number.isFinite(hook.timeout) && hook.timeout > 0 ? Math.min(hook.timeout, 300) : 30
        const child = spawn("sh", ["-lc", hook.command], {
          cwd, env: process.env, detached: process.platform !== "win32",
          stdio: ["ignore", "pipe", "pipe"],
        })
        let size = 0
        let settled = false
        let failure
        let forceTimer
        const finish = (error) => {
          if (settled) return
          settled = true
          clearTimeout(timer)
          clearTimeout(forceTimer)
          child.stdout.destroy()
          child.stderr.destroy()
          running.delete(child)
          if (error) reject(error)
          else resolve()
        }
        const stop = (message) => {
          if (settled || failure) return
          failure = new Error(message)
          try {
            if (process.platform !== "win32" && child.pid) process.kill(-child.pid, "SIGKILL")
            else child.kill("SIGKILL")
          } catch { child.kill("SIGKILL") }
          forceTimer = setTimeout(() => finish(failure), 1000)
        }
        running.set(child, stop)
        const timer = setTimeout(() => stop("Autopus hook timed out"), timeout * 1000)
        const count = (chunk) => {
          size += chunk.length
          if (size > OUTPUT_LIMIT) stop("Autopus hook output limit exceeded")
        }
        child.stdout.on("data", count)
        child.stderr.on("data", count)
        child.on("error", () => finish(new Error("Autopus hook process failed to start")))
        child.on("close", (code) => finish(failure || (code === 0 ? undefined : new Error("Autopus hook failed with exit code " + code))))
      })
    }

    async function runHooks(hooks, cwd) {
      for (const hook of hooks) await runCommand(hook, cwd)
    }

    function callKey(event) {
      if (disposed || typeof event.sessionID !== "string" || !event.sessionID || typeof event.id !== "string" || !event.id) {
        throw new Error("Autopus shell hook identity unavailable")
      }
      return JSON.stringify([event.sessionID, event.id])
    }

    async function resolveCwd(event) {
      const session = await bounded(ctx.session.get({ sessionID: event.sessionID }), 5000)
      const directory = session?.location?.directory
      if (session?.id !== event.sessionID || typeof directory !== "string" || !path.isAbsolute(directory) || directory.includes("\0")) {
        throw new Error("Autopus shell session directory unavailable")
      }
      const workdir = event.input?.workdir
      if (workdir !== undefined && (typeof workdir !== "string" || workdir.includes("\0"))) {
        throw new Error("Autopus shell working directory invalid")
      }
      return workdir ? path.resolve(directory, workdir) : directory
    }

    try {
      registrations.push(await ctx.tool.hook("execute.before", async (event) => {
        if (event.tool !== "shell") return
        const key = callKey(event)
        if (contexts.size >= 1024 || contexts.has(key)) throw new Error("Autopus shell hook context limit or duplicate call")
        contexts.set(key, null)
        try {
          const cwd = await resolveCwd(event)
          if (disposed) throw new Error("Autopus hook plugin is disposed")
          await runHooks(BEFORE_HOOKS, cwd)
          contexts.set(key, cwd)
        } catch (error) {
          contexts.delete(key)
          throw error
        }
      }))
      registrations.push(await ctx.tool.hook("execute.after", async (event) => {
        if (event.tool !== "shell") return
        const key = callKey(event)
        const cwd = contexts.get(key)
        contexts.delete(key)
        if (!cwd) throw new Error("Autopus shell hook has no verified before context")
        await runHooks(AFTER_HOOKS, cwd)
      }))
    } catch (error) {
      await Promise.all(registrations.map(registration => registration.dispose()))
      throw error
    }
    return async () => {
      disposed = true
      contexts.clear()
      for (const stop of running.values()) stop("Autopus hook plugin unloaded")
      await Promise.all(registrations.map(registration => registration.dispose()))
    }
  },
}
`
