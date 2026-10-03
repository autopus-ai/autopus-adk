package scaffold

import "fmt"

// BrowserGUIJourneyPackID is the pack that runs compiled @journey scenarios.
const BrowserGUIJourneyPackID = "browser-gui-journey"

// journeyTag selects the specs `auto qa scenario compile` emits for v2
// scenarios that act on the page. It mirrors scenario.JourneyTag; the scaffold
// does not import the scenario package to keep init free of compiler deps.
const journeyTag = "@journey"

// journeyGrepArgv selects the @journey subset with the same npm separator
// asymmetry exploreGrepArgv documents.
func journeyGrepArgv(packageManager string) []string {
	command := nodeCommand(packageManager)
	args := []string{"test"}
	if command == "npm" {
		args = append(args, "--")
	}
	return jsRunnerArgv(command, "playwright", append(args, "--grep", journeyTag)...)
}

// browserGUIJourneyPackExample renders the pack for intent-anchored journeys.
//
// It runs under the plain playwright adapter in browser-staging, never under
// gui-explore: journey specs click and fill by design, and the gui-explore
// guard would block them. The capture fixture no-ops outside a gui-explore
// run, so the same compiled specs stay runnable here without capture policy.
func browserGUIJourneyPackExample(signals projectSignals) string {
	return fmt.Sprintf(`# Example browser-staging Journey Pack for compiled @journey scenarios.
# Copy it into .autopus/qa/journeys/%[1]s.yaml and review before executing.
# Specs come from 'auto qa scenario compile' over qamesh.scenario.v2 files whose
# expected values trace to SPEC acceptance criteria or a confirmed recording.
# 'auto qa loop --lane browser-staging' runs this pack, triages failures, and
# repairs them on its own branch without moving any expected value.
id: %[1]s
title: Browser user journeys
surface: frontend
lanes: [browser-staging]
adapter:
  id: playwright
command:
  argv: [%[2]s]
  cwd: .
  timeout: 300s
checks:
  - id: %[1]s
    type: deterministic
    expected:
      exit_code: 0
source_refs:
  source_spec: SPEC-QALOOP-001
  acceptance_refs:
    - AC-QALOOP-018
  owned_paths:
    - src/**
  do_not_modify_paths:
    - .autopus/specs/**
    - .autopus/qa/scenarios/**
    - .codex/**
    - .opencode/**
    - .autopus/plugins/**
`, BrowserGUIJourneyPackID, renderInlineStrings(journeyGrepArgv(signals.PackageManager)))
}

func captureReadmeJourneySubset(signals projectSignals) []string {
	rows := []string{
		"### The journey subset",
		"",
		"Scenarios that click, fill, or submit compile with `@journey` instead of",
		"`@explore` and run under the `playwright` adapter in `browser-staging`. Copy this",
		"into `.autopus/qa/journeys/" + BrowserGUIJourneyPackID + ".yaml`:",
		"",
	}
	rows = append(rows, fencedYAML(browserGUIJourneyPackExample(signals))...)
	return append(rows,
		"",
		"Generate journeys from acceptance criteria with `auto qa scenario generate --spec",
		"<SPEC-ID> --agent claude`, record them with `auto qa record`, then `auto qa scenario",
		"promote --all` and `auto qa scenario compile`.",
		"",
	)
}
