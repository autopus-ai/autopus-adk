package companionmanifest

import (
	"strings"
	"testing"
)

const releaseSourceValidatorScript = "scripts/companion-release/validate-source.sh"

func TestReleaseSourceValidator_PhasesAcceptAnnotatedPredecessorDescendantAndExactPins(t *testing.T) {
	for _, phase := range releasePhases {
		if phase.acceptedField == "" {
			continue
		}
		t.Run(phase.phase, func(t *testing.T) {
			dir := cloneCurrentReleaseRepository(t)
			sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
			tree := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD^{tree}"))
			runGit(t, dir, "tag", "-am", phase.phase+" release candidate", phase.tag)
			output, err := runReleaseSourceValidator(t, dir, phase.tag, sha,
				"COMPANION_SOURCE_PIN_REQUIRED=1",
				"COMPANION_APPROVED_SOURCE_COMMIT="+sha,
				"COMPANION_APPROVED_SOURCE_TREE="+tree,
			)
			if err != nil {
				t.Fatalf("annotated pinned %s rejected: %v\n%s", phase.phase, err, output)
			}
			accepted := phase.acceptedField + "=" + sha
			if phase.acceptedField == "source-tree" {
				accepted = phase.acceptedField + "=" + tree
			}
			if !strings.Contains(output, "release-phase="+phase.phase) ||
				!strings.Contains(output, accepted) {
				t.Fatalf("validated %s output = %q", phase.phase, output)
			}
		})
	}
}

func TestReleaseSourceValidator_PhasesRejectInvalidIdentity(t *testing.T) {
	for _, phase := range releasePhases {
		if phase.rejects != "identity" {
			continue
		}
		prior := phase.priorPhase(t)
		t.Run(phase.phase, func(t *testing.T) {
			t.Run("lightweight", func(t *testing.T) {
				dir := cloneCurrentReleaseRepository(t)
				sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
				runGit(t, dir, "tag", phase.tag)
				output, err := runReleaseSourceValidator(t, dir, phase.tag, sha)
				if err == nil ||
					!strings.Contains(output, phase.phase+" release tag must be annotated") {
					t.Fatalf("lightweight %s result: %v\n%s", phase.phase, err, output)
				}
			})
			t.Run("missing_"+prior, func(t *testing.T) {
				dir, sha := newMinimalSourceRepository(t)
				runGit(t, dir, "tag", "-am", "orphan "+phase.phase, phase.tag)
				output, err := runReleaseSourceValidator(t, dir, phase.tag, sha)
				if err == nil ||
					!strings.Contains(output, "does not contain the immutable "+prior+" release") {
					t.Fatalf("%s-free %s result: %v\n%s", prior, phase.phase, err, output)
				}
			})
			t.Run("unapproved_source", func(t *testing.T) {
				dir := cloneCurrentReleaseRepository(t)
				sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
				tree := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD^{tree}"))
				runGit(t, dir, "tag", "-am", phase.phase+" release candidate", phase.tag)
				output, err := runReleaseSourceValidator(t, dir, phase.tag, sha,
					"COMPANION_SOURCE_PIN_REQUIRED=1",
					"COMPANION_APPROVED_SOURCE_COMMIT="+strings.Repeat("a", 40),
					"COMPANION_APPROVED_SOURCE_TREE="+tree,
				)
				if err == nil || !strings.Contains(output,
					"release commit differs from the approved exact source commit") {
					t.Fatalf("unapproved %s source result: %v\n%s", phase.phase, err, output)
				}
			})
		})
	}
}

func TestReleaseSourceValidator_PhasesRejectUnsignedTagWhenProductionSignatureIsRequired(t *testing.T) {
	for _, phase := range releasePhases {
		if phase.rejects != "unsignedTag" {
			continue
		}
		t.Run(phase.phase, func(t *testing.T) {
			dir := cloneCurrentReleaseRepository(t)
			sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
			tree := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD^{tree}"))
			runGit(t, dir, "tag", "-am", "unsigned "+phase.phase+" release candidate", phase.tag)
			environment := []string{
				"COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED=1",
				"COMPANION_SOURCE_PIN_REQUIRED=1",
				"COMPANION_APPROVED_SOURCE_COMMIT=" + sha,
				"COMPANION_APPROVED_SOURCE_TREE=" + tree,
			}
			if phase.phase == "A22" {
				environment = append(environment, "ADK_KEY_ROTATION_VERIFIED=1")
			}
			output, err := runReleaseSourceValidator(t, dir, phase.tag, sha, environment...)
			if err == nil || !strings.Contains(output,
				phase.phase+" release tag signature or R2 signer differs") {
				t.Fatalf("unsigned production %s result: %v\n%s", phase.phase, err, output)
			}
		})
	}
}

func TestReleaseSourceValidator_PhasesPinDirectPredecessorAncestor(t *testing.T) {
	source := readReleaseFile(t, releaseSourceValidatorScript)
	for _, phase := range releasePhases {
		t.Run(phase.phase, func(t *testing.T) {
			// 모든 좌표는 validate-source.sh의 phase 표에 정확히 한 번 등장한다.
			row := phase.tag + ") release_phase='" + phase.phase + "'"
			if strings.Count(source, row) != 1 {
				t.Fatalf("source validator phase row drifted: %s", row)
			}
			if phase.ancestorSHA == "" {
				return
			}
			prior := phase.priorPhase(t)
			pin := phase.phase + "_" + prior + "_ANCESTOR_SHA"
			declaration := "readonly " + pin + "='" + phase.ancestorSHA + "'"
			if strings.Count(source, declaration) != 1 {
				t.Fatalf("%s immutable %s ancestry pin drifted: %s", phase.phase, prior, declaration)
			}
			required := []string{
				`git cat-file -t "refs/tags/$GITHUB_REF_NAME"`,
				`git merge-base --is-ancestor "$` + pin + `" "$GITHUB_SHA"`,
				`[[ "$tag_object_type" == 'tag' ]]`,
				"fail '" + phase.phase + " source does not contain the immutable " + prior + " release'",
				"COMPANION_APPROVED_SOURCE_COMMIT", "COMPANION_APPROVED_SOURCE_TREE",
			}
			required = append(required, phase.extraSourceGates...)
			for _, want := range required {
				if !strings.Contains(source, want) {
					t.Fatalf("%s source gate missing %q", phase.phase, want)
				}
			}
		})
	}
}

func TestReleaseSourceValidator_A30RejectsCrossTagAnnotatedObjectReplay(t *testing.T) {
	dir := cloneCurrentReleaseRepository(t)
	sha := strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
	runGit(t, dir, "tag", "-am", "A29 replay object", "v0.50.118")
	tagObject := strings.TrimSpace(runGit(t, dir, "rev-parse", "refs/tags/v0.50.118"))
	runGit(t, dir, "update-ref", "refs/tags/v0.50.119", tagObject)
	output, err := runReleaseSourceValidator(t, dir, "v0.50.119", sha)
	if err == nil || !strings.Contains(output, "annotated tag object, type, or name headers differ") {
		t.Fatalf("cross-tag A29 object replay result: %v\n%s", err, output)
	}
}
