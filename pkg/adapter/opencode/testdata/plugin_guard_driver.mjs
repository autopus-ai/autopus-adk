// Replays tool calls through a generated Autopus plugin under a fake OpenCode
// host and prints one result per call as a JSON array (SPEC-EDITGUARD-001 S9).
// usage: node plugin_guard_driver.mjs <plugin> <v1|v2> <session-directory> <calls.json>
// A call is {"event": <V2 execute.before event>, "mode": <stub mode>?}; V1
// receives the same event as tool.execute.before(input, {args}).
import fs from "node:fs/promises"
import { pathToFileURL } from "node:url"

const [pluginPath, version, directory, callsPath] = process.argv.slice(2)
const calls = JSON.parse(await fs.readFile(callsPath, "utf8"))
const plugin = await import(pathToFileURL(pluginPath))
const shellTool = version === "v1" ? "bash" : "shell"
let before, after, cleanup
if (version === "v1") {
  const hooks = await plugin.default({ directory, worktree: directory })
  const input = (event) => ({ tool: event.tool, sessionID: event.sessionID, callID: event.id })
  before = (event) => hooks["tool.execute.before"](input(event), { args: event.input })
  after = (event) => hooks["tool.execute.after"](input(event), { args: event.input })
} else {
  const callbacks = new Map()
  cleanup = await plugin.default.setup({
    location: { directory: "/wrong/plugin-instance-location" },
    session: { get: async ({ sessionID }) => ({ id: sessionID, location: { directory } }) },
    tool: { hook: async (name, callback) => { callbacks.set(name, callback); return { dispose: async () => {} } } },
  })
  before = (event) => callbacks.get("execute.before")(event)
  after = (event) => callbacks.get("execute.after")({ ...event, status: "completed", result: {} })
}
const results = []
for (const call of calls) {
  if (call.mode !== undefined) await fs.writeFile(process.env.GUARD_STUB_MODE, call.mode)
  const started = Date.now()
  let result = { outcome: "resolved" }
  try {
    await before(call.event)
    if (call.event.tool === shellTool) await after(call.event)
  } catch (error) {
    result = { outcome: "rejected", message: error instanceof Error ? error.message : String(error) }
  }
  results.push({ ...result, ms: Date.now() - started })
}
if (cleanup) await cleanup()
console.log(JSON.stringify(results))
