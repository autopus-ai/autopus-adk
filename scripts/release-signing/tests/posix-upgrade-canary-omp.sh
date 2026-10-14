#!/bin/sh
# Sourced by posix-upgrade-canary.sh: the OMP agent surface a candidate
# generates. It uses that script's fail, assert_contains, MODE, NEW_VERSION,
# and PROJECT.

# From v0.50.126 the default autopus mode generates these agent definitions
# again (SPEC-MODELUX-003); v0.50.118 through v0.50.125 reused OMP's bundled
# agents and generated none.
AUTOPUS_AGENTS_SINCE=0.50.126
AUTOPUS_AGENTS='annotator architect debugger deep-worker devops executor explorer frontend-specialist
perf-engineer planner reviewer security-auditor spec-writer tester ux-validator validator'

# version_at_least VALUE MINIMUM compares two validated x.y.z versions.
version_at_least() {
    value_major=${1%%.*} value_rest=${1#*.}
    minimum_major=${2%%.*} minimum_rest=${2#*.}
    value_minor=${value_rest%%.*} value_patch=${value_rest#*.}
    minimum_minor=${minimum_rest%%.*} minimum_patch=${minimum_rest#*.}
    if [ "$value_major" -ne "$minimum_major" ]; then
        [ "$value_major" -gt "$minimum_major" ]
    elif [ "$value_minor" -ne "$minimum_minor" ]; then
        [ "$value_minor" -gt "$minimum_minor" ]
    else
        [ "$value_patch" -ge "$minimum_patch" ]
    fi
}

# candidate_generates_agents reports a candidate that generates the Autopus
# agent definitions. The fixture candidate is built from this source.
candidate_generates_agents() {
    [ "$MODE" = fixture ] || version_at_least "$NEW_VERSION" "$AUTOPUS_AGENTS_SINCE"
}

# assert_autopus_agent_surface checks the autopus-mode OMP surface: every
# Autopus agent definition is manifest-owned and named and described by its
# content, with no model line, since routes go to task.agentModelOverrides,
# and no retired @autopus_ alias; .omp/config.yml disables every bundled
# agent but task under a claim, not the retired marker policy. A name the
# installed omp does not bundle is an inert entry.
assert_autopus_agent_surface() {
    python3 - "$PROJECT" "$AUTOPUS_AGENTS" <<'PY' || fail 'the Autopus OMP agent surface is not the v0.50.126 shape'
import json
import pathlib
import sys

project = pathlib.Path(sys.argv[1])
manifest = json.loads((project / ".autopus/omp-manifest.json").read_text(encoding="utf-8"))
files = manifest.get("files", {})
for agent in sys.argv[2].split():
    relative = ".omp/agents/" + agent + ".md"
    path = project / relative
    if not path.is_file():
        raise SystemExit("missing agent definition: " + relative)
    text = path.read_text(encoding="utf-8")
    lines = text.split("\n")
    if lines[0] != "---" or "---" not in lines[1:]:
        raise SystemExit(relative + " has no frontmatter")
    front = lines[1:lines.index("---", 1)]
    keys = {line.split(":", 1)[0] for line in front if line and not line[0].isspace()}
    if "name: " + agent not in front:
        raise SystemExit(relative + " is not named " + agent)
    if not any(line.startswith("description: ") and line[len("description: "):].strip() for line in front):
        raise SystemExit(relative + " has no description")
    if "model" in keys:
        raise SystemExit(relative + " still pins a model in its frontmatter")
    if "@autopus_" in text:
        raise SystemExit(relative + " still names a retired @autopus_ alias")
    if files.get(relative, {}).get("policy") != "always":
        raise SystemExit(relative + " is not manifest-owned")
if files.get(".omp/config.yml", {}).get("policy") != "always":
    raise SystemExit(".omp/config.yml is not recorded under the claim")
PY
    assert_contains "$PROJECT/.omp/config.yml" 'disabledAgents:'
    for bundled in scout security-reviewer sonic designer librarian; do
        assert_contains "$PROJECT/.omp/config.yml" "- $bundled"
    done
}
