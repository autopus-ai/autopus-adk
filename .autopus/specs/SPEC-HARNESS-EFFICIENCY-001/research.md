# Research and existing primitives

The source-reviewed research is in `docs/harness-assessment.md`. Relevant findings
are selective skill utility, induced procedure cost, and separation of metadata
exposure from actual body loading. These motivate measurements, not an automatic
three-skill limit or reduced acceptance standards.

Reuse `ResolveCatalogSkillState`, `IsCoreSkill`, `LoadPreview`, existing skill
registry metadata, `gates.Decide`/input closures, telemetry usage normalization
and accepted-task summaries. Keep the Ultra promotion evaluator separate from
three-arm observational reporting because its existing shared config identity
must not be reinterpreted as a per-arm harness config identity.

Base commit: 4282114a. Existing uncommitted README/research and promptlayer
identity fixes belong to this conversation. Generated .omp edits predate this
upgrade and remain untouched. The installed CLI is not authoritative for source
checks: the local changelog records the 0.50.117 PATH skew, so build the candidate.
