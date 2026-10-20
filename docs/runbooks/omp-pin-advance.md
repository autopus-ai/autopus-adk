# Advancing the pinned OMP release

`oh-my-pi` ships often. Between 2026-08-29 and 2026-09-03 it moved v17.2.7 →
v18.0.11 → v18.1.2 → v18.1.4 → v18.1.5. The release pin does not follow
automatically, and this is the procedure that moves it.

## What the pin is, and what it is not

The pin has two halves, both declared in
`scripts/companion-release/prepare-release.sh`:

- `expected_omp_sha256` — the bytes
- the `omp/N.N.N` version, repeated at six more sites that cannot derive it

It is **not** what enforces the protocol contract. `initializeManaged` in
`internal/cli/pipeline_omp_context_active_rpc.go` negotiates managed protocol
v2, turns auto-retry and auto-compaction off, refuses a runtime whose idle state
still reports compaction enabled, and refuses one without native image
compaction — all against the binary that actually runs.

The pin's job is narrower and still worth keeping: before the release starts, it
declares exactly which executable the signed evidence will name. That is a
supply-chain pre-commitment a reviewer can read in the repository.

## Move it with one command

```bash
scripts/release-tools/advance-omp-pin.sh 18.1.5 --dry-run   # inspect first
scripts/release-tools/advance-omp-pin.sh 18.1.5
```

The script refuses to touch a file until the candidate has proven itself:

1. downloads `omp-darwin-arm64` from the immutable upstream release
2. measures the digest rather than trusting a typed one
3. requires the asset to self-report `omp/<version>`
4. runs the RPC handshake and requires `supportedProtocolVersions` to contain
   `2` — the same version `initializeManaged` negotiates

Then it rewrites all seven sites and prints the verification list.

`go test ./internal/companionmanifest/ -run TestOMPPin` fails the moment the
sites disagree, so a half-moved pin is a test failure rather than a discovery
made while the release canary is already running.

## The one thing the script cannot check

The compaction reduction floor. `min_reduction_basis_points` is measured, not
declared: the 40-call cohort in `--apply` computes the actual reduction and
fails evidence generation if the new OMP compacts less than the floor.

`--preflight` does not measure it. The `canary_records:42,provider_calls:40`
fields in the preflight receipt are a declared plan, and the script exits before
the cohort block runs. Running the cohort by hand means reproducing the release
lane's gateway, credential locator, policy digest and producer identity, and the
evidence it produced would be unsigned and unusable for a release anyway.

So the floor is validated where it is measured:

1. advance the pin
2. run the release script tests and `preflight-release.sh`
3. start the release with `release-prep.sh --apply`
4. if the cohort fails the floor, revert the pin with
   `advance-omp-pin.sh <previous>` and release on the previous pin

A failed cohort costs the coordinate, so treat step 4 as expected rather than
exceptional when the OMP major version moves.

## Measured 2026-09-03: omp/18.1.5 is incompatible

The v0.50.114 attempt advanced the pin to omp/18.1.5, passed every local gate,
passed CI, passed preflight with zero failures, and then failed closed inside
the cohort:

```
observe-session call 6 failed closed: managed active OMP manual compaction response is invalid
error_code=runtime_failed error_stage=call failed_sequence=6
transcript_records=7/42
```

**No tag was created and the coordinate survived.** The canary runs before
tagging, which is why a wrong pin costs 25 minutes and 6 provider calls rather
than a coordinate.

What this proved: `snapcompact-image-schema=omp-v17.2.7` in the active policy
identity is not decoration. The manual compaction response shape is
version-coupled, and 18.1.5 changed it. `negotiate_protocol` still advertises
v2, so the handshake probe in `advance-omp-pin.sh` cannot see the difference —
manual compaction needs a live provider session with history to exercise.

So the script now refuses 18.1.5 by name with that observation, and the honest
boundary is stated rather than implied: **the handshake probe proves the launch
contract, the cohort proves the compaction contract.** Anything the probe cannot
reach costs one cohort run to learn, once.

### Narrowed by measurement, 2026-09-03

Both versions return the identical no-op response on an empty session, so the
no-op branch is not the difference:

```
17.2.7: {"command":"compact","success":false,"error":"Nothing to compact (session too small)"}
18.1.5: {"command":"compact","success":false,"error":"Nothing to compact (session too small)"}
```

That leaves three candidates inside the real compaction path in
`pipeline_omp_context_active_lifecycle.go`, all reachable only with a session
that has enough history to actually compact:

1. `success` is false with an error text the no-op branch does not match, so the
   frame falls through to the manual check and is rejected for `!frame.Success`.
2. the pre or post compaction ACK never arrived, so `preACKed`/`postACKed` is
   false when the response lands.
3. `data.summary` is absent or renamed, so `validPipelineOMPActiveManualResult`
   returns false.

Two of the three were eliminated by measurement on 2026-09-03, at no provider
cost, by comparing the two binaries directly:

| Checked | 17.2.7 | 18.1.5 | Verdict |
|---|---|---|---|
| `"summary"` in compaction result | present | present | candidate 3 eliminated |
| `Nothing to compact (session too small)` | present | present | no-op text unchanged |
| `Nothing to compact (no messages yet)` | present | present | second no-op text, matched by neither harness branch |
| `snapcompact`, `auto_compaction_end`, `compactionSummary` | present | present | frame vocabulary unchanged |
| `session_before_compact`, `session_compact` hooks | present | present | ACK hook names unchanged |
| `negotiate_protocol` v2 | advertised | advertised | launch contract unchanged |

Three live attempts to reach a real compaction cost about ten provider calls and
all returned `Nothing to compact (session too small)`: short prose over three or
four turns does not approach the threshold, even with `contextWindow` lowered to
16384 in the experiment's own models.yml. The cohort reaches compaction because
it runs real tasks with tool output over many turns, which is the workload, not
a threshold trick.

So the instrument was fixed instead. The rejection in
`pipeline_omp_context_active_lifecycle.go` now names the failing gate:

```
managed active OMP manual compaction response is invalid: id_match=%t command=%q
success=%t already_responded=%t pre_acked=%t post_acked=%t summary_valid=%t error=%q
```

It stays body-free — no transcript, no summary text, only which gate failed.
The next 18.1.5 cohort attempt yields the answer in one run rather than none.

One experiment settles it: drive a managed session against 18.1.5 with enough
turns to trigger compaction and capture the `compact` response frame verbatim.
That costs real provider calls, which is why it belongs to the adaptation task
rather than to a pin move. `workflow_context_runtime_managed_rpc_product.go`
already prints `id`, `want`, and `success` on failure, so the product path is
the cheaper instrument if it reproduces.

Adapting the harness to 18.1.5's compaction response is a separate task. It
changes the evidence oracle, so it needs its own cohort run and its own
`min_reduction_basis_points` measurement.

## Diagnosed 2026-09-03: the refusal, and the gate behind it

The fixed instrument answered in one standalone cohort run, six provider calls:

```
managed active OMP manual compaction response is invalid:
  id_match=true command="compact" success=false already_responded=false
  pre_acked=true post_acked=false summary_valid=false
  error="snapcompact would not reduce context locally."
```

`strings` over the staged binaries settles where it came from:

| Message | 17.2.7 | 18.1.2 | 18.1.5 |
|---|---|---|---|
| `snapcompact would not reduce context locally.` | absent | 6 | 6 |

So 18.1.x added a refusal class. It is not a protocol violation — it means the
history is already minimal — and the harness knew only
`Nothing to compact (session too small)`. 18.1.x also decides this *after*
firing the pre-compaction hook, so the refusal lands with the pre-ACK already
acknowledged, which the no-op branch also rejected.

Both are fixed in `pipeline_omp_context_active_lifecycle.go`: all three measured
refusals are recognized, and the pre-ACK is tolerated while the post-ACK must
still be absent. The no-op proof is unchanged — it re-reads the transcript and
idle state and requires both untouched.

Measured result: the standalone cohort against 18.1.5 now completes **42/42**
instead of stopping at 6.

### But 18.1.x still cannot ship, and the reason is the oracle

The run now fails one step later, at `OMP context promotion cohort gates
failed`. That gate requires, among other things, `compactionCount >= 2` and
`MedianReductionBasisPoints >= MinReductionBasisPoints`. If 18.1.x declines
every compaction as unprofitable, there are zero compactions and no reduction to
attest, so the promotion evidence has nothing to say.

That is a real product boundary, not a bug to adapt away. Advancing to 18.1.x
needs one of:

1. a cohort workload that gives 18.1.x history worth compacting, or
2. an oracle that accepts "declined because it would not help" as a distinct,
   attestable outcome rather than requiring reduction.

The second changes what the promotion evidence claims, so it is a policy
decision with its own SPEC, not a pin move.

The adaptation above is kept regardless: a legitimate refusal should never read
as a protocol violation, 17.2.7 never emits the new message so its behaviour is
unchanged, and the failure now lands on the honest gate.

## Settled 2026-09-03: 18.1.5 measurably does not reduce context

The same plan generator, the same 20 task pairs, the same gateway and model, run
back to back with only the OMP binary swapped:

| | omp/17.2.7 | omp/18.1.5 |
|---|---|---|
| calls completed | 40 | 40 |
| compaction cycles | **8** | **2** |
| pre/post ACKs | 8 / 8 | — |
| pairs, AB/BA balance | 20, 10/10 | 20, 10/10 |
| integrity / security / quality failures | 0 / 0 / 0 | 0 / 0 / 0 |
| fallback, rollback verified | true, true | true, true |
| **median reduction** | **passed 2000 bp** | **0 bp** |
| verdict | gates pass | one gate fails |

The gate diagnostic reports it exactly:

```
OMP context promotion cohort gates failed: pairs=20/20 ab=10/10 ba=10/10
observed_ab=10 observed_ba=10 compactions=2/2 integrity_failures=0
security_failures=0 quality_regressions=0 fallback_verified=true
rollback_verified=true median_reduction_bp=0/2000
```

`compactions=2/2` met the minimum. Everything else passed. The only failure is
`median_reduction_bp=0/2000`: the compactions 18.1.5 does perform reduce context
by zero basis points in the metric the promotion evidence attests, while 17.2.7
compacts four times as often and clears the 20% floor on the same histories.

### What this closes

**Advancing the pin to 18.1.x is refused on merit, not on a harness gap.**

The workload option is dead: 18.1.5 ran the identical workload that gives 17.2.7
eight compactions and a passing median. There is nothing to make heavier.

The oracle option is worse than it looked: relaxing the floor would not admit a
version that reduces slightly less, it would admit one that reduces **zero** in
the attested metric. `MinReductionBasisPoints != 2000` is a hard constant in
`omp_context_promotion_report_verify.go` precisely so that this decision cannot
be made by editing a policy field.

So this is an upstream question, not a harness change: 18.1.x's snapcompact
either does something different or produces a shape the measurement no longer
sees as reduction. Until that is answered upstream, `advance-omp-pin.sh` refuses
18.1.5 by name and the pin stays at omp/17.2.7.

The refusal-class adaptation is kept regardless. It is correct on its own terms —
a legitimate refusal is not a protocol violation, 17.2.7 never emits the new
message, and without it the failure hides behind a misleading error instead of
landing on `median_reduction_bp=0/2000`.

## The pin governs evidence; the runtime is the operator's own OMP

Two things drift apart and it is worth being explicit about which one the pin
holds.

`prepare-release.sh` pins the OMP that **produces the promotion evidence**. The
pipeline at runtime resolves whatever OMP the operator has installed —
`observePipelineOMPVersion` compares it to its own model receipt, never to the
release pin. So a machine can run omp/18.1.10 against evidence measured on
omp/17.2.7, and nothing refuses it.

That is not a lie in the evidence: the report names the version and the
executable digest it measured. What was missing is any signal to the operator,
and the gap was live — this machine runs omp/18.1.10, every doctor capability
row passed, and the compaction benefit measured zero.

So `auto doctor` now carries the measured verdict:

```
compaction.measured_reduction  supported=false  required=false
                               reason=measured_zero_reduction
```

Three states, kept apart on purpose:

| reason | meaning |
|---|---|
| `measured_reduction_verified` | this repository measured the floor cleared |
| `measured_zero_reduction` | measured, and it delivers 0 bp |
| `reduction_unmeasured` | never measured here — unknown, not broken |

The row is advisory rather than required, because an unmeasured version is not a
failure. Claiming otherwise would be a claim this repository cannot support.

### The refusal is a table now, not a version number

`advance-omp-pin.sh` first refused `18.1.5` by name, which would have let
18.2.0 walk straight past it into another cohort run. Upstream ships every few
days, so the guard is a verdict table with the same three states, and clearing a
refusal is a measurement rather than an edit:

```
17.2.7            -> in use, measured good
18.1.2 | 18.1.5   -> refused with compactions=2/2 median_reduction_bp=0/2000
18.1.13           -> refused with compactions=2/2 median_reduction_bp=895/2000
anything else     -> refused as unmeasured; --measure moves it for one cohort
```

## Measured 2026-09-07: omp/18.1.13 compacts, but under half the floor

The A29 (`v0.50.118`) release attempt ran the cohort on omp/18.1.13 with the
same plan generator, the same 20 task pairs, the same gateway and the same
model (`gpt-5.6-sol`) as the passing omp/17.2.7 runs. It completed 42/42
records and failed at the evidence gate; no tag or remote state was created:

```
pairs=20/20 ab=10/10 ba=10/10 observed_ab=10 observed_ba=10 compactions=2/2
integrity_failures=0 security_failures=0 quality_regressions=0
fallback_verified=true rollback_verified=true median_reduction_bp=895/2000
```

| | omp/17.2.7 | omp/18.1.5 | omp/18.1.13 |
|---|---|---|---|
| compaction cycles | 8 | 2 | 2 |
| median reduction | passed 2000 bp | 0 bp | **895 bp** |
| every other gate | pass | pass | pass |

Two things this settles. The 0 bp on 18.1.5 was real and upstream fixed part of
it: v18.1.8 ("context compaction incorrectly accepting archived history that
was larger because of opaque reasoning data") moved the same workload from
0 bp to 895 bp. And 18.1.x still compacts a quarter as often and clears under
half the floor, so the pin stays at omp/17.2.7 on merit: the promotion
evidence attests a 20% median reduction, and 18.1.13 delivers 9%.

One hypothesis worth carrying to upstream rather than into the oracle: since
v18.1.6 Codex "provider-native compaction continues to use catalog-selected
Responses Lite" — if 18.1.x hands part of the reduction to remote compaction,
the local transcript measurement would not see it. That is a question for
`can1357/oh-my-pi`, with the three-column table above as the evidence.

The first 18.1.13 attempt lost its verdict: the error frame was body-free by
contract and the transcript died with the temp dir. The frame now carries
`gate_diagnostic` (counts and basis points only) and the canary failure
receipt prints it, so a cohort costs one run to learn, not two.

## Measuring an unmeasured version

The "standalone cohort" above was the release canary with the pin moved. It is
not a separate instrument: the cohort needs the release lane's gateway,
credential locator, policy identity (which embeds the pin), producer identity,
and the isolated-UID sandbox, and the policy identity lives in Go code, so the
pin must move before anything can be measured. The guard therefore has an
explicit measurement mode:

```bash
scripts/release-tools/advance-omp-pin.sh 18.1.13 --dry-run   # launch contract only
scripts/release-tools/advance-omp-pin.sh 18.1.13 --measure   # move an unmeasured pin
go test ./internal/cli/ -run TestWorkflowContextImplementationIdentityCommand  # re-pin the digest
scripts/release-tools/advance-release-coordinate.sh <from> <A> <to> <B>
scripts/release-tools/release-prep.sh --apply                # the measurement
```

The cohort runs before the tag, the evidence tag, and every remote mutation, so
a failing verdict leaves nothing behind. Either way, move the number into the
verdict table in `advance-omp-pin.sh`: a pass makes the version the pin in use,
a fail makes it a refused row. Versions already measured bad stay refused even
under `--measure`; a new cohort is worth its 40 provider calls only when
upstream shipped a change that plausibly alters compaction. For 18.1.13 that
change is v18.1.8: "Fixed context compaction incorrectly accepting archived
history that was larger because of opaque reasoning data, allowing the next
compaction strategy to run instead" — a candidate explanation for the 0 bp
measured on 18.1.5.

## Why not drop the pin and measure at release time

Because then nothing in the repository states which executable the evidence will
name until after it is signed. The evidence records the measured version and
digest either way, but a reviewer would have to trust the run instead of reading
the declaration. The churn is a smaller cost than losing that.

## Why not alarm on drift in CI

Upstream ships every few days. A CI job that fails on a stale pin would make CI
red on someone else's schedule. `preflight-release.sh` warns instead, and now
names the exact command:

```
warn  upstream latest is v18.1.5; the pin is v17.2.7. Advancing is its own task
      because the reduction floor is measured in the canary, not declared: run
      scripts/release-tools/advance-omp-pin.sh 18.1.5
```

## Moved 2026-10-20: omp/18.12.3, unmeasured, for gpt-6.1-sol

The A36 (`v0.50.126`) canary on omp/17.2.7 with `gpt-5.6-sol` stopped at call 4
with `managed active OMP usage delta is empty`. It was not the pin and not an ADK
regression: v0.50.125, which passed on 2026-10-12, failed the same way that day.
Four distinct single-turn `gpt-5.6-sol` requests sent back to back through the
gateway answered three times and then returned
`Codex is at capacity, please try again later (code=turn_rate_limit_exceeded)`,
and canary call 4 is the fourth turn inside about ten seconds. Gateway 18.2.10
turned that error into an empty `stop` with zero usage, and 17.2.7 retried the
empty stop and dropped it, which is why the run reported usage, not the limit.

`gpt-6.1-sol` did not hit the limit, but the 17.2.7 catalog has no
`gpt-6.1-sol`, so the release model needs a newer pin. omp/18.12.3 passed the
launch contract (`--measure`: asset digest `f2a657b3…`, managed protocol v2),
ran the unchanged managed RPC path through a 40-call cohort against a loopback
fake gateway, and passed a live 12-call prefix of the cohort with
`gpt-6.1-sol`:

| pair | full input | optimized input | optimized compacted |
|---|---|---|---|
| 1 | 29891 | 29891 | no |
| 2 | 54143 | 50352 | yes |
| 3 | 78395 | 74604 | no |
| 4 | 102647 | 76482 | yes |
| 5 | 126899 | 100734 | no |
| 6 | 151151 | 75600 | yes |

18.12.3 compacts every second optimized call, including the one-turn session
17.2.7 refused with "Nothing to compact", so the gap widens with history. That
is a trend, not the verdict: the reduction floor is measured by the next
`release-prep.sh --apply`. Record its numbers in the `advance-omp-pin.sh`
table and in `ompContextReductionVerified` either way.
