// usage: node plugin_v2_rules.mjs <plugin> <cases.json>
// Drives the generated V2 plugin's conditional rule delivery with a stubbed
// host. cases.json carries the RE2 verdicts the Go test computed, so the same
// commands must fire the same rules under the JavaScript regex engine.
import assert from "node:assert/strict"
import fs from "node:fs/promises"
import { pathToFileURL } from "node:url"

const [pluginPath, casesPath] = process.argv.slice(2)
const plugin = (await import(pathToFileURL(pluginPath))).default
const cases = JSON.parse(await fs.readFile(casesPath, "utf8"))
const callbacks = new Map()
const cleanup = await plugin.setup({
  location: { directory: "/unused" },
  session: {
    get: async ({ sessionID }) => ({ id: sessionID, location: { directory: "/unused" } }),
    hook: async (name, callback) => { callbacks.set("session." + name, callback); return { dispose: async () => {} } },
  },
  tool: { hook: async (name, callback) => { callbacks.set(name, callback); return { dispose: async () => {} } } },
})
const before = callbacks.get("execute.before")
const context = callbacks.get("session.context")
assert.equal(typeof context, "function")

async function systemFor(sessionID) {
  const event = { sessionID, system: [{ type: "text", text: "host" }], messages: [] }
  await context(event)
  return event.system.slice(1).map((part) => part.text).join("\n")
}
let call = 0
async function shell(sessionID, command) {
  await before({ tool: "shell", sessionID, id: "call-" + call++, input: { command } })
}

// Each case runs in a fresh session: the rules its command fires are exactly
// the ones the Go regexp engine fires, plus any fail-closed rule.
for (const item of cases.commands) {
  const sessionID = "session-" + call
  await shell(sessionID, item.command)
  const text = await systemFor(sessionID)
  for (const rule of cases.rules) {
    const expected = cases.always.includes(rule) || item.fires.includes(rule)
    assert.equal(text.includes("## Rule: " + rule + "\n"), expected, JSON.stringify({ command: item.command, rule, text }))
  }
}

// A fired rule stays in every later generation of its session only.
await shell("session-a", cases.fireCommand)
assert.ok((await systemFor("session-a")).includes(cases.fireRule))
assert.ok((await systemFor("session-a")).includes(cases.fireRule))
assert.ok(!(await systemFor("session-b")).includes(cases.fireRule))

// A non-shell tool, a missing command, and a missing session fire nothing.
await before({ tool: "read", sessionID: "session-c", id: "read", input: { command: cases.fireCommand } })
await shell("session-c", undefined)
await before({ tool: "shell", sessionID: "", id: "no-session", input: { command: cases.fireCommand } }).catch(() => {})
assert.ok(!(await systemFor("session-c")).includes(cases.fireRule))

// A host event without a system array is left alone instead of throwing.
await context({ sessionID: "session-a" })

if (cases.omitted) {
  await shell("session-big", cases.fireCommand)
  const text = await systemFor("session-big")
  assert.ok(text.includes("> Omitted to fit the context budget: " + cases.omitted), text)
  assert.ok(new TextEncoder().encode(text).length <= cases.limit + 512)
}

await cleanup()
console.log("RULES_PASS")
