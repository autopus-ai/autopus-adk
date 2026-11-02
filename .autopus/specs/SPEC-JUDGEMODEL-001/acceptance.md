# SPEC-JUDGEMODEL-001 Acceptance Criteria

Every scenario runs over a temporary project. RL is SPEC-REVMODEL-001's fixture with the default CLI providers (claude `[--print, --model, claude-fable-5-1, --effort, max]`, codex with `model_policy: quality`, gemini on agy) and judge claude; R3 is its fixture with three OMP providers (claude `anthropic/claude-fable-5-1:max`) and judge claude, read against the strict catalog CATG. Review rounds run on a recording backend; no provider binary runs.

## Test Scenarios

### S1: AC-JM-001 - The keys load, validate, and stay absent by default
Priority: Must
Given an `autopus.yaml` whose review gate sets `judge: claude`, `judge_model: claude-opus-5-5`, and `judge_effort: high`
When the configuration is loaded and saved again
Then `JudgeModel` is `claude-opus-5-5`, `JudgeEffort` is `high`, and the saved file holds both keys
And a saved default configuration holds neither `judge_model` nor `judge_effort`
And `judge_model` without `judge` fails with `spec.review_gate.judge_model needs spec.review_gate.judge`, `judge_effort` without `judge_model` with `spec.review_gate.judge_effort needs spec.review_gate.judge_model`, `judge_model: -x` and `judge_model: "a b"` with `spec.review_gate.judge_model must not start with "-" or hold whitespace or control characters`, and `judge_mode1: x` with the strict decoder's unknown-key error (test `TestReviewGateJudgeModel_LoadsValidatesAndRoundTrips`).

### S2: AC-JM-002 - The judge request carries the judge model, reviewers keep theirs
Priority: Must
Given RL with judge claude, `judge_model: claude-opus-5-5`, `judge_effort: high`, and the reviewers claude and codex
When one review round runs on the recording backend
Then the reviewer request of claude carries `--model claude-fable-5-1 --effort max`, the judge request carries `--model claude-opus-5-5 --effort high`, and every other argv element of the two is equal, `--tools=Read,Grep,Glob` last
And with judge codex, `judge_model: gpt-6-luna`, `judge_effort: high` the judge argv carries `-m gpt-6-luna` and `model_reasoning_effort="high"` with `ModelPolicy` pinned, while the codex reviewer argv keeps its own values
And with judge gemini on agy and `judge_model: gemini-3.1-pro-high` the judge argv appends `--model gemini-3.1-pro-high` and the reviewer argv has no `--model`
And with R3, judge claude, `judge_model: anthropic/claude-opus-5-5`, `judge_effort: max` the judge request's `Config.Model` is `anthropic/claude-opus-5-5:max` and the reviewer's stays `anthropic/claude-fable-5-1:max`
And with no `judge_model`, or with `--judge codex` while the key names claude's model, the judge config is exactly the reviewer config, as before, and without the key the receipt's judge object holds no `model` or `effort` key (tests `TestSpecReviewJudgeModel_JudgeRequestCarriesItsModel`, `TestSpecReviewJudgeModel_AssemblyPerBackend`, `TestSpecReviewJudgeModel_AbsentOrOtherJudgeKeepsTheReviewerConfig`).

### S3: AC-JM-003 - A judge model the backend cannot run is refused before any call
Priority: Must
Given RL with judge claude, or R3 with judge claude
When the review assembles its judge with each value below
Then planning fails before the readiness probe and the backend run, with an error starting `spec review: spec.review_gate.judge_model` or `spec review: spec.review_gate.judge_effort` that names the value and the judge:

| judge backend | judge_model | judge_effort | error contains |
|---|---|---|---|
| claude CLI | `anthropic/claude-opus-5-5` | | `is an OMP model selector, but judge claude runs on the CLI backend (claude)` |
| claude CLI | `claude-opus-5-5` | `turbo` | `is not one of low, medium, high, xhigh, max` |
| OMP | `claude-opus-5-5` | `max` | `is not a <provider>/<model> selector, which judge claude on the OMP backend needs` |
| OMP | `openai-codex/gpt-6-astra` | `high` | `belongs to the openai family, but judge claude belongs to anthropic` |
| OMP | `anthropic/claude-opus-5-5:max` | `high` | `already names thinking max` |
| agy | `gemini-3.1-pro` | `high` | `agy ids carry their effort` |
| agy | `claude-opus-5-5` | | `is not an agy id of judge gemini's family google` |
| unknown binary `foo` | `m1` | | `whose model flag Autopus does not know` |

And the expected value of the readiness probe's call count and of the backend's request count is zero (test `TestSpecReviewJudgeModel_RefusedBeforeAnyCall`).

### S4: AC-JM-004 - The plan block and the receipt name the judge model
Priority: Must
Given RL with judge claude, `judge_model: claude-opus-5-5`, `judge_effort: high`
When the plan block is printed and a review writes its receipt
Then the expected stdout of the block holds `  judge: claude (always) — spec.review_gate.judge` followed by `  judge model: claude-opus-5-5 · high — spec.review_gate.judge_model`, and the receipt's judge holds `"model": "claude-opus-5-5"` and `"effort": "high"`
And with `--judge codex` the block reads `  judge model: codex의 리뷰어 model — spec.review_gate.judge_model은 judge claude에만 씁니다` and the receipt's judge has neither field
And without `judge_model` the block has no `judge model:` line and the receipt has neither field
And with claude on the OMP backend, each of `judge_model: anthropic/claude-opus-5-5` with `judge_effort: max`, `judge_model: anthropic/claude-opus-5-5:max` alone, and `judge_model: anthropic/claude-opus-5-5:max` with `judge_effort: max` gives the plan line `  judge model: anthropic/claude-opus-5-5:max — spec.review_gate.judge_model`, the receipt's `"model": "anthropic/claude-opus-5-5"` and `"effort": "max"`, and the judge request's `Config.Model` `anthropic/claude-opus-5-5:max` (tests `TestSpecReviewJudgeModel_PlanLine`, `TestSpecReviewJudgeModel_ReceiptFields`, `TestSpecReviewJudgeModel_OMPFormsNormalize`).

### S5: AC-JM-005 - The command sets, changes, and clears the judge model
Priority: Must
Given R3 written to disk with an inline comment `# arbiter` on the judge line and English text
When `auto quality review judge claude --model anthropic/claude-opus-5-5 --effort max --dry-run` runs
Then the expected stdout is `Settings that change:`, `  spec.review_gate.judge_model reviewer model → anthropic/claude-opus-5-5`, `  spec.review_gate.judge_effort none → max`, the judge model note, and `--dry-run: nothing written.`, and the file is unchanged
When it runs without `--dry-run`
Then the file is the original with `        judge_model: anthropic/claude-opus-5-5` and `        judge_effort: max` appended after `        doc_context_max_lines: 200`, the comment kept, and the loader value carries both
When `auto quality review judge claude --model default` runs
Then the change lines read `judge_model anthropic/claude-opus-5-5 → reviewer model` and `judge_effort max → none`, and the file is byte-identical to the original
And after setting the model again, `auto quality review judge codex` clears both keys while changing the judge, and `auto quality review judge none` clears them while writing `judge: ""`
And on RL `auto quality review judge claude --model claude-opus-5-5` writes `judge_effort: max`, the claude default, and a later `--model claude-haiku-4-5` removes the `judge_effort` line (tests `TestQualityReviewJudge_SetsAndClearsTheJudgeModel`, `TestQualityReviewJudge_CLIEffortDefault`).

### S6: AC-JM-006 - The command refuses what the tab refuses and shows the keys
Priority: Must
Given R3 with English text
When `judge claude --effort max`, `judge none --model x`, `judge claude --model default --effort max`, `judge claude --model anthropic/claude-opus-9`, and `judge claude --model x` over a `judge_model: ${JUDGE_MODEL}` file run
Then each exits 1 with `--effort needs --model`, `judge none takes no judge model`, `--model default takes no --effort`, `The installed OMP catalog does not attest claude judge model anthropic/claude-opus-9 (accepted: anthropic/claude-fable-5-1 (high, low, max, medium), anthropic/claude-opus-5-5 (high, max))`, and `judge_model of spec.review_gate comes from an environment variable; this tab leaves it: ${JUDGE_MODEL}`, and writes nothing
And `auto quality review show` prints `spec.review_gate.judge_model = anthropic/claude-opus-5-5` and `spec.review_gate.judge_effort = max` right after the judge line, `--json` carries `"judge_model"` and `"judge_effort"` with those values, and a file without the keys prints and serializes exactly as before
And for `judge_model: anthropic/claude-opus-5-5:max` with `judge_effort: max`, show prints both keys as written while the menu summary ends `judge claude (anthropic/claude-opus-5-5:max)` (tests `TestQualityReviewJudge_Refusals`, `TestQualityReviewJudge_ShowListsTheJudgeModel`, `TestReviewJudgeModel_InlineThinkingShowsOnce`).

### S7: AC-JM-007 - The tab picks the judge model of the kept judge
Priority: Must
Given RL on disk with the Korean session and claude on PATH, a file that sets no judge model key, so the new option appears although the keys are absent (outside the byte identity of the Outcome Lock)
When the operator keeps participation and models, chooses `claude — judge model 고르기` (option 4), `claude-opus-5-5`, and effort `high`, skips the live check, and applies
Then the judge select reads `judge \n1. 그대로 — claude\n2. codex\n3. gemini\n4. claude — judge model 고르기\nEnter`, the model select `claude judge model \n1. 그대로 — 리뷰어 model과 같음\n2. claude-fable-5-1\n3. claude-opus-5-5\n4. claude-sonnet-5-5\n5. claude-haiku-4-5\nEnter`, and the effort select `claude judge effort \n1. low\n2. medium\n3. high\n4. xhigh\n5. max\nEnter`
And the preview lists `  spec.review_gate.judge_model 리뷰어 model → claude-opus-5-5` and `  spec.review_gate.judge_effort 없음 → high`, the note, and the judge cell `judge (claude-opus-5-5 · high) *`
And the file gains exactly the two key lines at the end of `spec.review_gate`, the next summary line ends `judge claude (claude-opus-5-5 · high)`, and the assembled judge argv carries `--model claude-opus-5-5 --effort high` (test `TestReviewTab_JudgeModelForTheKeptJudge`).

### S8: AC-JM-008 - A new judge asks its model, and the reviewer model clears it
Priority: Must
Given R3 on disk with the Korean session and CATG
When the operator chooses judge `codex`, then `openai-codex/gpt-6-luna` and thinking `high`, and applies
Then the judge line changes to `codex` and the file gains `judge_model: openai-codex/gpt-6-luna` and `judge_effort: high`
When the operator then chooses `codex — judge model 고르기` and `리뷰어 model과 같음`, and applies
Then the model select reads `codex judge model \n1. 그대로 — openai-codex/gpt-6-luna:high\n2. 리뷰어 model과 같음\n3. openai-codex/gpt-6-astra\n4. openai-codex/gpt-6-luna\nEnter`, the preview lists `judge_model openai-codex/gpt-6-luna → 리뷰어 model`, and both key lines are gone
And cancelling the confirmation, keeping the judge, and keeping its judge model write nothing (tests `TestReviewTab_NewJudgeAsksItsModelAndClears`, `TestReviewTab_JudgeModelKeepAndCancelWriteNothing`).

### S9: AC-JM-009 - A placeholder judge model is left alone
Priority: Must
Given R3 whose review gate sets `judge_model: ${JUDGE_MODEL}` with `JUDGE_MODEL=anthropic/claude-opus-5-5`
When the tab reaches the judge
Then it prints `spec.review_gate의 judge_model 값이 환경 변수라 이 탭에서 바꾸지 않습니다: ${JUDGE_MODEL}` instead of the judge select, and the expected value of the file after the pass is the original byte for byte (test `TestReviewTab_PlaceholderJudgeModelIsLeft`).

### S10: AC-JM-010 - Documentation and scope
Priority: Must
Given the branch
When the documents and the change set are read
Then `auto quality --help`, `auto quality review --help`, `README.md`, `docs/README.ko.md`, and the CHANGELOG's unreleased section each name `spec.review_gate.judge_model`, `judge_effort`, and `SPEC-JUDGEMODEL-001`; the three skill sources name `judge_model`; every path `git diff --name-only origin/main` lists is in Module ownership, the test files of Generated And Changed Surfaces included; and no path under `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.omp/`, or `.agents/` changed (test `TestJudgeModelDocs`; scope by `git diff --name-only origin/main` against Module ownership).

## Oracle Acceptance Notes

Every oracle is a fixed string, argv, file text, or count. File oracles compare with the original text plus or minus the named lines, so any other changed byte fails. The recording backend returns a valid reviewer or judge answer, so a round reaches the judge.
