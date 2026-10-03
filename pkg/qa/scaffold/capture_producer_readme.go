package scaffold

import "strings"

// captureReadmeBody is assembled line by line because the snippets need fenced
// code blocks, and a Go raw string literal cannot contain a backtick.
func captureReadmeBody(signals projectSignals) string {
	rows := captureReadmeIntro()
	rows = append(rows, captureReadmeJourneyPack(signals)...)
	rows = append(rows, captureReadmeRuntime()...)
	rows = append(rows, desktopObservationReadme(signals)...)
	return strings.Join(rows, "\n") + "\n"
}

func captureReadmeIntro() []string {
	return []string{
		"# Autopus QAMESH GUI capture producer",
		"",
		"`auto qa run` never drives Playwright. It allocates a capture directory, writes the",
		"effective `gui.capture` policy from your Journey Pack, and reads back one",
		"`capture-index.json`. The two `.cjs` files here are the producer that writes it, and",
		"this project owns them.",
		"",
		"## Wiring",
		"",
		"1. Register the reporter and hand tracing to the fixture in `playwright.config.ts`:",
		"",
		"```ts",
		"import path from 'node:path';",
		"import { defineConfig } from '@playwright/test';",
		"",
		"const captureDir = process.env.AUTOPUS_QAMESH_GUI_CAPTURE_DIR;",
		"",
		"export default defineConfig({",
		"  reporter: [['list'], ['./.autopus/qa/capture/autopus-capture.reporter.cjs']],",
		"  // Videos are only collected when the `video` stream is declared. Playwright",
		"  // writes them under outputDir, so point that at the capture directory.",
		"  outputDir: captureDir ? path.join(captureDir, 'videos') : 'test-results',",
		"  use: {",
		"    // The fixture starts and stops tracing itself; leaving Playwright's own",
		"    // trace mode on would make both fight over the same browser context.",
		"    trace: 'off',",
		"    video: 'off',",
		"  },",
		"});",
		"```",
		"",
		"2. Import the extended `test` in every spec that should be captured:",
		"",
		"```ts",
		"import { test, expect } from '../.autopus/qa/capture/autopus-capture.fixture.cjs';",
		"```",
		"",
		"CommonJS specs use `require` instead:",
		"",
		"```js",
		"const { test, expect } = require('../.autopus/qa/capture/autopus-capture.fixture.cjs');",
		"```",
		"",
		"The fixture needs a `page`, so import it in browser specs only. API-only specs",
		"should keep importing `@playwright/test` directly.",
		"",
	}
}

// captureReadmeJourneyPack carries the gui-explore Journey Pack as an example
// instead of `auto qa init` generating it. A generated pack was measured to be
// unrunnable on every real project, and `gui-explore` is a `must` lane, so the
// generated version turned the release gate red. The examples are validated by
// TestCaptureReadmeEmbedsValidatingGUIExplorePacks: a doc snippet nobody checks is
// how the same trap comes back.
func captureReadmeJourneyPack(signals projectSignals) []string {
	rows := []string{
		"## The gui-explore Journey Pack",
		"",
		"`auto qa run` and `auto qa explore` need a project-local `gui-explore` Journey Pack,",
		"and `auto qa init` deliberately does not generate one. A generated pack cannot pass",
		"until this project has a read-only exploration subset: `forbidden_actions` includes",
		"`mutation`, so a pack aimed at the whole suite is blocked on the first `click`, and a",
		"pack whose selection is empty violates the capture contract's minimum-one-step rule.",
		"`gui-explore` is a `must` lane in the default `prelaunch` profile, so an unrunnable",
		"pack would turn the release gate red. Until this project writes one, `auto qa explore`",
		"reports a setup gap - status `warning`, exit code 0 - rather than failing.",
		"",
		"Copy this into `.autopus/qa/journeys/browser-gui-explore.yaml`, then narrow",
		"`allowed_origins`, `command.argv`, and `source_refs` to this repo. `allowed_origins`",
		"below is the baseURL detected in this project's Playwright config, or the Vite",
		"dev-server origin when the config states none as a plain string:",
		"",
	}
	rows = append(rows, fencedYAML(browserGUIExplorePackExample(signals))...)
	rows = append(rows,
		"",
		"A desktop shell uses `.autopus/qa/journeys/desktop-gui-explore.yaml` instead, because",
		"its webview is served by the framework dev server rather than by a Playwright baseURL:",
		"",
	)
	rows = append(rows, fencedYAML(desktopGUIExplorePackExample(signals))...)
	rows = append(rows, captureReadmeExploreSubset()...)
	return append(rows, captureReadmeJourneySubset(signals)...)
}

func captureReadmeExploreSubset() []string {
	return []string{
		"",
		"### The read-only subset",
		"",
		"The pack runs the `@explore`-tagged subset only, and the spec has to import the",
		"fixture to be captured. One spec is enough to make the journey pass:",
		"",
		"```ts",
		"import { test, expect } from '../.autopus/qa/capture/autopus-capture.fixture.cjs';",
		"",
		"test('checkout renders @explore', async ({ page }) => {",
		"  await page.goto('/checkout');",
		"  await expect(page.getByRole('heading', { name: 'Checkout' })).toBeVisible();",
		"});",
		"```",
		"",
		"A spec that clicks, fills, or submits must not carry `@explore`. `forbidden_actions`",
		"includes `mutation`, so the GUI runtime blocks click, dblclick, tap, fill, press,",
		"check, uncheck, selectOption, setInputFiles, and dragTo, and the journey is then",
		"reported `blocked` rather than failed.",
		"",
		"Only `mutation` and those exact method names are enforced. The guard observes",
		"Playwright method names, never business intent, so a label like `payment` or",
		"`email_send` blocks nothing. Declaring one is allowed but is reported as",
		"`unenforceable_forbidden_actions` in the `gui-policy-runtime` check evidence, so",
		"the pack never claims a guarantee the harness does not provide.",
		"",
		"`gui.network_policy.mode` is enforced by the guard, not just validated:",
		"`summary-only` observes without stopping anything, `local-only` aborts every",
		"request whose origin is outside `allowed_origins`, and `blocked` also aborts",
		"xhr/fetch to an allowed origin so the UI renders from static assets with no",
		"data traffic. Aborted requests appear as `network_stopped` in the",
		"`gui-policy-runtime` check and never fail the journey — the declared policy",
		"working is not a violation of it. Pick `summary-only` if your specs need",
		"third-party assets such as hosted fonts.",
		"",
	}
}

func captureReadmeRuntime() []string {
	return []string{
		"## Runtime contract",
		"",
		"| Environment variable | Meaning |",
		"| --- | --- |",
		"| `AUTOPUS_QAMESH_GUI_CAPTURE_POLICY_PATH` | Effective `gui.capture` policy as JSON. Absent means this is not a QAMESH run and both files no-op. |",
		"| `AUTOPUS_QAMESH_GUI_CAPTURE_DIR` | Capture directory. Every `ref` in the index is relative to it. |",
		"| `AUTOPUS_QAMESH_GUI_CAPTURE_INDEX_PATH` | Where `capture-index.json` must be written. |",
		"",
		"Layout inside the capture directory:",
		"",
		"```text",
		"shards/<step>.json       one per test, written by the fixture",
		"screenshots/<step>.png   per the screenshot cadence",
		"traces/<step>.zip        when the trace stream is declared",
		"capture-index.json       merged by the reporter in onEnd",
		"```",
		"",
		"## Retention",
		"",
		"Raw screenshots, traces, and videos stay local-only under `.autopus/qa/runs/**` and",
		"are never published. The index carries digests, sizes, and derived references only:",
		"network URLs are reduced to `origin:<n>/path`, and the values typed into `fill` or",
		"`press` are never recorded. The harness publishes the sanitized",
		"`capture-index.published.json` as the `capture_index` evidence artifact.",
		"",
		"## Editing",
		"",
		"`auto qa init` never overwrites these files. Two rules keep a local edit working:",
		"emit only the keys the contract defines, because the Go decoder rejects unknown keys",
		"and drops the whole index; and keep `totals` exactly consistent with `steps` and",
		"`media`, because a mismatch is treated as a wrong report rather than a rounding",
		"error.",
	}
}

// fencedYAML wraps a rendered pack in a Markdown block. The pack ends with a
// newline, which would otherwise split into an empty row before the closing fence
// and leave a blank line inside the block.
func fencedYAML(body string) []string {
	rows := []string{"```yaml"}
	rows = append(rows, strings.Split(strings.TrimRight(body, "\n"), "\n")...)
	return append(rows, "```")
}
