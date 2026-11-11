# Rules for advance-release-coordinate-apply.pl: every file a coordinate move
# touches, and how it moves.
#
#   shift      - rewrite the predecessor to FROM and FROM to TO on every line
#                that no other rule claims
#   shift_only - shift only lines matching these patterns
#   keep       - history lines left alone; each must match at least once, so a
#                keep that silently stops protecting anything is an error
#   clone      - an accumulating entry: the FROM entry stays, and a shifted
#                copy of it is inserted right after it as the TO entry
#   append     - literal [find, replace, expected count] edits for phase lists
#
# Patterns are Perl regexes; {FROM_PHASE}, {FROM_TAG}, {PREV_PHASE} and the
# rest are replaced with quoted values. A keep names the history it protects
# by content (the incident), never by version, because the next move would
# otherwise shift a version-named keep along with everything else.
#
# This list was derived from the A45 -> A46 move (795a40f9) and proved against
# it: replaying A45 -> A46 with these rules reproduces that commit's code files.
my @shift_files = qw(
  .github/workflows/homebrew-formula-bridge-recovery.yaml
  .github/workflows/release.yaml
  .github/workflows/upgrade-canary.yaml
  internal/companionmanifest/release_contract_test.go
  internal/companionmanifest/release_current_evidence_fixture_test.go
  internal/companionmanifest/release_current_evidence_test.go
  internal/companionmanifest/release_current_signature_evidence_test.go
  internal/companionmanifest/release_homebrew_formula_bridge_cask_test.go
  internal/companionmanifest/release_homebrew_formula_bridge_test.go
  internal/companionmanifest/release_homebrew_formula_recovery_test.go
  internal/companionmanifest/release_lineage_phase_source_test.go
  internal/companionmanifest/release_omp_context_evidence_test.go
  internal/companionmanifest/release_upgrade_canary_test.go
  pkg/companionmanifest/release_signing_wiring_test.go
  scripts/companion-release/build-omp-context-candidate.sh
  scripts/companion-release/prepare-release.sh
  scripts/companion-release/publish-homebrew-formula-bridge.sh
  scripts/companion-release/publish-release-coordinates.sh
  scripts/companion-release/tests/release-current-repository-test.sh
  scripts/companion-release/tests/release-current-signature-hardening-test.sh
  scripts/companion-release/tests/release-lineage-pins-hardening-test.sh
  scripts/companion-release/tests/release-omp-context-evidence-hardening-test.sh
  scripts/companion-release/tests/release-prep-environment-inheritance-test.sh
  scripts/companion-release/tests/release-prep-hardening-test.sh
  scripts/companion-release/tests/testdata/mock-release-prep-gh.sh
  scripts/companion-release/tests/testdata/mock-tap-gh.sh
  scripts/companion-release/verify-current-release-signatures.sh
  scripts/companion-release/verify-current-release.sh
  scripts/companion-release/verify-omp-context-evidence-tag.sh
  scripts/companion-release/verify-omp-context-release-binary.sh
  scripts/companion-release/verify-release-prep-lock.sh
  scripts/release-tools/preflight-release.sh
);
my $phase_list = q~|| "$release_phase" == '{FROM_PHASE}' ]]~;
my $phase_list_next = q~|| "$release_phase" == '{FROM_PHASE}' || "$release_phase" == '{TO_PHASE}' ]]~;

[
  (map { { file => $_, shift => 1 } } @shift_files),
  # The v0.51.4 ruleset was the 34th and the readers saw only 30 (#319).
  { file => 'scripts/companion-release/verify-release-tag-ruleset.sh', shift => 1,
    keep => ['ruleset was the 34th'] },
  { file => 'scripts/companion-release/tests/release-tag-ruleset-pagination-test.sh', shift => 1,
    keep => ['release ruleset was the 34th'] },
  # The v0.51.4 final tap ref read lagged the PATCH (#320).
  { file => 'scripts/companion-release/tests/release-homebrew-hardening-test.sh', shift => 1,
    keep => ['the Cask was published at 08:13:08'] },
  # Every shipped receipt triple stays; the TO row follows the FROM row.
  { file => 'scripts/companion-release/tests/release-hardening-test.sh', shift => 1,
    keep => [q~^contains "\$producer_receipt" 'v~],
    clone => [{ anchor => q~^contains "\$producer_receipt" '{FROM_TAG} {FROM_VERSION} {FROM_PHASE}'$~ }] },
  { file => 'scripts/companion-release/produce-public-key-receipt.sh',
    clone => [{ anchor => q~^{FROM_TAG} {FROM_VERSION} {FROM_PHASE}$~ }] },
  # The phase map, its ancestor pin, and the frozen policy only ever grow.
  { file => 'scripts/companion-release/validate-source.sh',
    clone => [
      { anchor => q~^readonly {FROM_PHASE}_{PREV_PHASE}_ANCESTOR_SHA=~ },
      { anchor => q~^\s+{FROM_TAG}\) release_phase='{FROM_PHASE}' ;;$~ },
      { anchor => q~^\s+{FROM_PHASE}\) git merge-base --is-ancestor~ },
    ],
    append => [[$phase_list, $phase_list_next, 2],
      [q~/{FROM_PHASE} policy'~, q~/{FROM_PHASE}/{TO_PHASE} policy'~, 1]] },
  # The FROM declaration is the predecessor declaration shifted forward with
  # the measured pins; the TO branch is the FROM branch shifted forward.
  { file => 'scripts/companion-release/verify-public-key-lineage-coordinates.sh',
    clone => [
      { anchor => q~^readonly {PREV_PHASE}_REPOSITORY=~, down => q~^readonly {PREV_PHASE}_ARM64_MANIFEST_SHA256=~ },
      { anchor => q~^elif \[\[ "\$GITHUB_REF_NAME" == "\${FROM_PHASE}_TAG"~, down => q~^  prior_tag_object=~ },
    ],
    append => [[q~/{FROM_PHASE} policy'~, q~/{FROM_PHASE}/{TO_PHASE} policy'~, 1]] },
  { file => 'scripts/companion-release/verify-public-key-lineage.sh',
    append => [[$phase_list, $phase_list_next, 3]] },
  { file => 'internal/companionmanifest/release_lineage_shipped_phases_test.go',
    clone => [{ anchor => q~^\t\tphase: "{FROM_PHASE}", tag: "{FROM_TAG}"~, up => q~^\t\{$~, down => q~^\t\},$~ }] },
  { file => 'internal/companionmanifest/release_lineage_phase_matrix_test.go',
    shift_only => [q~^func TestLineageVerifier_A0Bootstraps~],
    clone => [{ anchor => q~\{name: "{FROM_PHASE}", tag: "{FROM_TAG}", message:~ }] },
  { file => 'internal/companionmanifest/release_lineage_test.go',
    append => [[q~/{FROM_PHASE} policy"~, q~/{FROM_PHASE}/{TO_PHASE} policy"~, 1]] },
];
