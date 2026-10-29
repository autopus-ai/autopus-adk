package opencode

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// openCodeRuleRuntime is the V2 plugin's conditional rule delivery, which
// mirrors the claude-code dispatcher (pkg/rulecond): a shell call whose command
// matches a rule condition fires the rule, and the rule text is advisory. On
// Claude Code the dispatcher injects the body once as tool context; OpenCode has
// no per-call context, so the `context` session hook puts every rule fired in
// the session into each later generation's system prompt. Probe 2026-10-29 on
// OpenCode 2.0.10 confirmed that hook reaches a plugin and that a pushed system
// part arrives in the provider request.
//
// A condition JavaScript cannot compile fails closed: that rule is delivered on
// every generation, because opencode.json no longer lists it. The tracker never
// throws, so a rule fault cannot fail a shell call or a generation.
var openCodeRuleRuntime = strings.NewReplacer(
	"__RULE_CONTEXT_LIMIT__", strconv.Itoa(rulecond.MaxContextBytes),
	"__RULE_HEADER__", jsString(rulecond.RuleHeaderPrefix),
	"__RULE_OMITTED__", jsString(rulecond.TruncationPrefix),
).Replace(`const RULE_SESSION_LIMIT = 1024
const RULE_CONTEXT_LIMIT = __RULE_CONTEXT_LIMIT__
const RULE_HEADER = __RULE_HEADER__
const RULE_OMITTED = __RULE_OMITTED__

function createRuleTracker(definitions) {
  const encoder = new TextEncoder()
  const size = (text) => encoder.encode(text).length
  const compiled = definitions.map((rule) => {
    try {
      return { name: rule.name, body: rule.body, conditions: rule.conditions.map((condition) => new RegExp(condition)), always: false }
    } catch {
      return { name: rule.name, body: rule.body, conditions: [], always: true }
    }
  })
  const fired = new Map()

  function note(event) {
    try {
      const sessionID = event?.sessionID
      const command = event?.input?.command
      if (typeof sessionID !== "string" || !sessionID || typeof command !== "string") return
      for (const rule of compiled) {
        if (rule.always || !rule.conditions.some((condition) => condition.test(command))) continue
        let names = fired.get(sessionID)
        if (!names) {
          if (fired.size >= RULE_SESSION_LIMIT) fired.delete(fired.keys().next().value)
          names = new Set()
          fired.set(sessionID, names)
        }
        names.add(rule.name)
      }
    } catch {}
  }

  // Whole rules are kept in name order while they fit RULE_CONTEXT_LIMIT, and
  // the rest are named in one notice, as the claude-code dispatcher does.
  function render(selected) {
    const kept = []
    const omitted = []
    let total = 0
    for (const rule of selected) {
      const block = RULE_HEADER + rule.name + "\n" + rule.body
      const cost = size(block) + (kept.length > 0 ? 2 : 0)
      if (total + cost > RULE_CONTEXT_LIMIT) {
        omitted.push(rule.name)
        continue
      }
      kept.push(block)
      total += cost
    }
    if (omitted.length > 0) kept.push(RULE_OMITTED + omitted.join(", "))
    return kept.join("\n\n")
  }

  function inject(event) {
    try {
      if (!Array.isArray(event?.system)) return
      const names = fired.get(event.sessionID)
      const selected = compiled.filter((rule) => rule.always || names?.has(rule.name))
      if (selected.length === 0) return
      event.system.push({ type: "text", text: render(selected) })
    } catch {}
  }

  return { note, inject, clear: () => fired.clear() }
}`)

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
