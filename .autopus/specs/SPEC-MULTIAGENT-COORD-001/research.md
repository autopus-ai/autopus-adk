# Evidence

See docs/multiagent-coordination.md for primary research and current platform
sources. The scaling v3 paper (2026-04-08) changes sample counts and statistics
from earlier search snippets. The August coordination study's shared-file
benefit is workload-dependent. Neither supports a universal agent count.

Local gaps: Phase.DependsOn was ignored by ParallelRunner; scheduling evidence
was computed before unordered slot contention; workerreceipt validated each path
but not ownership containment; OpenCode's generated --team text silently
substituted the default task pipeline. Default CLI execution remains sequential.

Baseline: 4282114a plus the preceding uncommitted 0.50.118 efficiency upgrade.
No remote merge, provider upgrade or release coordinate change is required.
