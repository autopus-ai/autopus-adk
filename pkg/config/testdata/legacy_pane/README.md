# legacy_pane fixtures (SPEC-PANERM-001 T3)

Inputs for the legacy config matrix (acceptance.md S4-S7, RFP-2).

| File | Content |
|------|---------|
| `c1.yaml` | C1: `autopus.yaml` written by binary O (`auto` v0.50.123 built from `c447badc`) via `auto init --platforms claude-code --yes`; group K paths = P1 |
| `c2.yaml` | C2: C1 edited to hold the five group K keys under `claude`, `codex`, `my-local`, plus `prompt_via_args`, `orchestra.subprocess.{max_concurrent,work_dir,rounds}`, `features.cc21.monitor_enabled`, `future_extension`, and `# keep-me`; group K paths = P2 |
| `c2-prime.yaml` | C2': C2 without its group K lines (S6 control) |
| `c2o.yaml` | C2o: C2 plus `operator_extension: {credential_ref: ${OMP_SECRET}}` (S7 e) |
| `c3.yaml` | C3: typo `pane_argz` on line 4 |
| `c4-scalar.yaml`, `c4-seq.yaml` | C4: non-mapping values on the `orchestra.providers.*` path |
| `c5-orchestra.yaml`, `c5-subprocess.yaml` | C5: `pane_args` at undeclared paths |
| `c6.yaml` | C6: C2 plus C3's line under `codex` |
| `c7.yaml` | C7: C2 with `platforms: [claude]`, which `MigratePlatformNames` rewrites to `claude-code` |

`legacy_pane_fixtures_test.go` checks the fixtures against P1 and P2 and pins the S4 and S5 behavior that holds at B.
The S7 writer oracles are red at B; they skip with a reason naming their owner task (T8) until it lands.
`AUTOPUS_PANERM_RED=1 go test ./pkg/config -run TestLegacyPaneFixtures` runs them anyway.
