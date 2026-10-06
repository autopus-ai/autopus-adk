import { spawn } from "node:child_process"

const BEFORE_HOOKS = [
  { command: "auto check --hygiene --arch --quiet --staged --warn-only", timeout: 30 }
]

const AFTER_HOOKS = [
  { command: "auto react check --quiet", timeout: 60 }
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
