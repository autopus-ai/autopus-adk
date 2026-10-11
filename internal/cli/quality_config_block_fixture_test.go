package cli

// Fixtures of SPEC-MODELUX-001 S13. Each file is split by hand into the bytes
// before the quality block, the block, and the bytes after it, so a test can
// check that a block write keeps the outer bytes without asking the writer
// where the block ends.

// The repository autopus.yaml at 35f68268 (fixture R): quality.default ultra,
// balanced HB2, ultra HU1, quality followed by design and workflow.
const blockRepoPrefix = `mode: full
project_name: autopus-adk
platforms:
    - claude-code
    - codex
    - antigravity-cli
    - opencode
    - omp
isolate_rules: true
omp_context_policy:
    profile: omp-active-probed
    profiles:
        omp-safe-default:
            history_mode: off
            memory_mode: off
            history_target_tokens: 1000
            fallback: canonical_full
            capability_policy: probe_required
            runtime_root_policy: isolated_task_owned
            mutation_scope: session_overlay
        omp-active-probed:
            history_mode: active
            memory_mode: off
            history_target_tokens: 1000
            fallback: canonical_full
            capability_policy: probe_required
            runtime_root_policy: isolated_task_owned
            mutation_scope: session_overlay
language:
    comments: en
    commits: ko
    ai_responses: ko
architecture:
    auto_generate: true
    enforce: true
    layers: []
    max_file_lines: 300
lore:
    enabled: true
    auto_inject: false
    required_trailers:
        - Constraint
    stale_threshold_days: 90
spec:
    id_format: SPEC-{DOMAIN}-{NUMBER}
    ears_types:
        - ubiquitous
        - event-driven
        - unwanted
        - optional
        - complex
    review_gate:
        enabled: true
        strategy: debate
        providers: [claude, codex, gemini]
        judge: claude
        max_revisions: 2
        auto_collect_context: true
        context_max_lines: 0
methodology:
    mode: tdd
    enforce: true
    review_gate: true
hooks:
    pre_commit_arch: true
    pre_commit_lore: true
    react_ci_failure: true
    react_review: true
session:
    handoff_enabled: true
    continue_file: .auto-continue.md
    max_context_tokens: 2000
orchestra:
    enabled: true
    default_strategy: consensus
    timeout_seconds: 240
    judge: claude
    providers:
        claude:
            backend: omp
            model: anthropic/claude-opus-5-5:max
            subprocess:
                timeout: 900
        codex:
            backend: omp
            model: openai-codex/gpt-6-astra:max
            subprocess:
                timeout: 900
        gemini:
            backend: omp
            model: google-antigravity/gemini-3.1-pro:high
            subprocess:
                timeout: 900
    commands:
        brainstorm:
            strategy: debate
            providers: [claude, codex, gemini]
        plan:
            strategy: consensus
            providers: [claude, codex, gemini]
        review:
            strategy: debate
            providers: [claude, codex, gemini]
        secure:
            strategy: consensus
            providers: [claude, codex, gemini]
`

const blockRepoQuality = `quality:
    default: ultra
    supervisor_model_policy: quality
    presets:
        balanced:
            description: 핵심 분석은 Opus, 기본 작업은 Sonnet. Haiku 미사용.
            agents:
                architect: opus
                debugger: sonnet
                devops: sonnet
                executor: sonnet
                explorer: sonnet
                planner: opus
                reviewer: sonnet
                security-auditor: opus
                spec-writer: opus
                tester: sonnet
                validator: sonnet
        ultra:
            description: 모든 에이전트를 Opus로 실행. 최고 품질.
            agents:
                architect: opus
                debugger: opus
                devops: opus
                executor: opus
                explorer: opus
                planner: opus
                reviewer: opus
                security-auditor: opus
                spec-writer: opus
                tester: opus
                validator: opus
`

const blockRepoSuffix = `design:
    enabled: true
    max_context_lines: 80
    inject_on_review: true
    inject_on_verify: true
    external_imports: false
workflow:
    coverage_threshold: 85
`

// The hand-written two-space file of probe A1 (fixture W): blank lines, an
// aligned trailing comment, and an anchor outside quality that a later key
// aliases. The blank line after the block belongs to the next key.
const blockHandPrefix = `# Project config (hand written)
mode: full
project_name: 'demo'   # trailing comment
platforms: [claude-code, codex]

# shared anchor block
future_extension:
  decoy: &bin codex

hooks:
  pre_commit_arch: true

`

const blockHandQuality = `quality:
  # comment inside quality
  default: balanced
  presets:
    balanced:
      description: "b"
      agents:
        planner: fable
    ultra:
      description: "u"
      agents:
        planner: fable
`

const blockHandSuffix = `
orchestra:
  providers:
    codex:
      binary: *bin
`

// The third probe A1 file: quality is the last key and the sections use
// different indentation widths.
const blockMixedPrefix = `mode: full
project_name: demo
platforms:
  - claude-code
hooks:
    pre_commit_arch: true
`

const blockMixedQuality = `quality:
  default: balanced
  presets:
    balanced:
      agents:
        planner: fable
`

// A column-one comment between quality children stays inside the block; the
// column-one comment right before the next key stays outside (MUX-R05).
const blockColumnPrefix = `project_name: demo
mode: full
platforms: [claude-code]
`

const blockColumnQuality = `quality:
  default: balanced
# preset notes
  presets:
    balanced:
      description: b
    ultra:
      description: u
`

const blockColumnSuffix = `# workflow notes
workflow:
  coverage_threshold: 85
`

// A file without a quality block.
const blockAbsentPrefix = `project_name: demo
mode: full
platforms: [claude-code]
`

// F1 is review finding F-001 verbatim: the quality block holds no ${, yet the
// loader-expanded value reaches balanced.description through the alias.
const blockLeakAlias = `project_name: &note '${PRESET_NOTE}'
mode: full
platforms: [claude-code]
quality:
  default: balanced
  presets:
    balanced:
      description: *note
    ultra:
      description: u
`

// F2 puts the placeholder directly into a quality value.
const blockLeakDirect = `project_name: demo
mode: full
platforms: [claude-code]
quality:
  default: balanced
  presets:
    balanced:
      description: b
    ultra:
      description: u
    fast:
      description: '${PRESET_NOTE}'
`

// F3 keeps both the anchor and its alias inside quality.
const blockInnerAlias = `project_name: demo
mode: full
platforms: [claude-code]
quality:
  default: balanced
  presets:
    balanced:
      description: &shared everyday
    ultra:
      description: *shared
`
