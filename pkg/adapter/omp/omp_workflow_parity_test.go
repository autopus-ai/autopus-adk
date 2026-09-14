package omp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMP002_WorkflowParity_GenerateEmitsCanonicalCommandsAndSkills(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	pf, err := a.Generate(context.Background(), configForOMP())
	require.NoError(t, err)

	want := make(map[string]bool, len(workflowSpecs))
	wantWorkflowTargets := make(map[string]bool, len(workflowSpecs))
	for _, spec := range workflowSpecs {
		want[spec.Name] = true
		wantWorkflowTargets[filepath.ToSlash(filepath.Join(".omp", "skills", spec.Name, "SKILL.md"))] = true
	}
	require.Len(t, want, 20, "workflowSpecs is the canonical twenty-name authority")
	workflow, err := a.prepareWorkflowSkillMappings(configForOMP())
	require.NoError(t, err)
	assert.Equal(t, wantWorkflowTargets, mappingTargetSet(workflow),
		"the workflow emitter, not the extended emitter, must own all twenty targets")
	extended, err := a.prepareExtendedSkillMappings(configForOMP())
	require.NoError(t, err)
	for _, file := range extended {
		assert.False(t, wantWorkflowTargets[filepath.ToSlash(file.TargetPath)],
			"extended emitter must exclude workflow target %q", file.TargetPath)
	}

	commands := map[string]bool{}
	commandBodies := map[string]string{}
	skills := map[string]string{}
	seen := map[string]int{}
	coordinationBodies := map[string]bool{
		"agent-pipeline":     false,
		"auto-go":            false,
		"worktree-isolation": false,
	}
	for _, file := range pf.Files {
		target := filepath.ToSlash(file.TargetPath)
		seen[target]++
		if strings.HasPrefix(target, ".omp/commands/") && strings.HasSuffix(target, ".md") {
			name := strings.TrimSuffix(strings.TrimPrefix(target, ".omp/commands/"), ".md")
			if !strings.Contains(name, "/") {
				commands[name] = true
				commandBodies[name] = string(file.Content)
			}
		}
		const skillPrefix, skillSuffix = ".omp/skills/", "/SKILL.md"
		if strings.HasPrefix(target, skillPrefix) && strings.HasSuffix(target, skillSuffix) {
			name := strings.TrimSuffix(strings.TrimPrefix(target, skillPrefix), skillSuffix)
			if want[name] {
				skills[name] = string(file.Content)
			}
		}
		if strings.HasPrefix(target, skillPrefix) && strings.HasSuffix(target, skillSuffix) {
			_, body := splitEmittedFrontmatter(t, string(file.Content))
			sweepBody := stripOMPIntentionalPlatformRootGlobs(body)
			for _, token := range ompWorkflowForbiddenTokens() {
				assert.NotContains(t, sweepBody, token, "generated body %s retained token %q", target, token)
			}
			assert.NotContains(t, body, "````json", "generated body %s contains a malformed nested JSON fence", target)
			if name := strings.TrimSuffix(strings.TrimPrefix(target, skillPrefix), skillSuffix); name == "agent-pipeline" || name == "auto-go" || name == "worktree-isolation" {
				coordinationBodies[name] = true
				if name == "agent-pipeline" || name == "worktree-isolation" {
					// A no-growth ratchet on the OMP-native core coordination skills:
					// injected into every OMP session, so the bound equals the current
					// emitted size and moves only when a runtime contract is genuinely
					// added. Raised from 392 for the compact change-contract path
					// (`auto spec change`, the `spec_authoring` gate, declared change
					// classes) plus merged final verification and `loop_status`.
					assert.LessOrEqual(t, strings.Count(body, "\n"), 423,
						"OMP-native core coordination skills must stay bounded")
					for _, token := range []string{
						"sonnet", "haiku", `model: "opus"`, "Opus-tier",
						"bypassPermissions", "acceptEdits", "--dangerously-skip-permissions", "Agent tool",
					} {
						assert.NotContains(t, body, token,
							"OMP-native core skill %s retained foreign execution policy %q", name, token)
					}
				}
			}
		}
	}
	for target, count := range seen {
		assert.Equal(t, 1, count, "emitted target %q must have duplicate count zero", target)
	}
	assert.Equal(t, map[string]bool{
		"agent-pipeline":     true,
		"auto-go":            true,
		"worktree-isolation": true,
	}, coordinationBodies, "all coordination-heavy generated bodies must be swept")
	assert.Equal(t, want, commands, "command basenames must equal workflowSpecs")
	assert.Equal(t, want, mappingContentKeys(skills),
		"workflow skill basenames must equal workflowSpecs; extended skills are excluded")
	assertOMPExecutionOwnerSurfaceParity(t, commandBodies, skills)

	router, ok := skills["auto"]
	if assert.True(t, ok, "the thin auto router skill must be emitted") {
		frontmatter, body := splitEmittedFrontmatter(t, router)
		assert.Contains(t, frontmatter, "name: auto")
		assert.Contains(t, body, "# Autopus 명령 라우터")
		assert.NotContains(t, body, "## Context Profile", "the router must not impersonate a detail skill")
	}
	for _, spec := range workflowSpecs[1:] {
		content, ok := skills[spec.Name]
		if !assert.True(t, ok, "detailed workflow skill %s must be emitted", spec.Name) {
			continue
		}
		frontmatter, body := splitEmittedFrontmatter(t, content)
		assert.Contains(t, frontmatter, "name: "+spec.Name)
		assert.NotEmpty(t, strings.TrimSpace(body))
		assert.Contains(t, body, "## ", "%s needs structured execution sections", spec.Name)
		assert.NotContains(t, body, "# Autopus 명령 라우터")
	}
}

func assertOMPExecutionOwnerSurfaceParity(t *testing.T, commands, skills map[string]string) {
	t.Helper()
	for _, surface := range []struct {
		name string
		body string
	}{
		{name: "auto router command", body: commands["auto"]},
		{name: "auto router skill", body: skills["auto"]},
		{name: "auto-go command", body: commands["auto-go"]},
		{name: "auto-go skill", body: skills["auto-go"]},
	} {
		assert.Contains(t, surface.body, "--execution-owner omp|orca", "%s lost the exact owner flag", surface.name)
		assert.NotContains(t, surface.body, "OMP-local", "%s retained a deprecated owner spelling", surface.name)
		assert.NotContains(t, surface.body, "Orca-supervised", "%s retained a deprecated owner spelling", surface.name)
	}
	for _, surface := range []struct {
		name string
		body string
	}{
		{name: "auto-status command", body: commands["auto-status"]},
		{name: "auto-status skill", body: skills["auto-status"]},
	} {
		assert.Contains(t, surface.body, "execution-owner", "%s lost receipt guidance", surface.name)
		assert.Contains(t, surface.body, "`hub`", "%s lost OMP-native hub guidance", surface.name)
		assert.Contains(t, surface.body, "user-session roots", "%s claims external session-root visibility", surface.name)
	}
}

func TestOMP002_RoutingContract_RouterAndAliasesResolveExactDetails(t *testing.T) {
	t.Parallel()

	a := NewWithRoot(t.TempDir())
	routerCommand, err := a.renderRouterCommand()
	require.NoError(t, err)
	routerSkill, err := a.renderRouterSkill()
	require.NoError(t, err)

	for _, spec := range workflowSpecs[1:] {
		spec := spec
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()
			alias, renderErr := a.renderWorkflowCommand(spec, configForOMP())
			require.NoError(t, renderErr)
			for _, entry := range []struct {
				name        string
				content     string
				wantNoFuzzy bool
			}{
				{name: "router command", content: routerCommand, wantNoFuzzy: true},
				{name: "router skill", content: routerSkill, wantNoFuzzy: true},
				{name: "direct alias", content: alias},
			} {
				t.Run(entry.name, func(t *testing.T) {
					assert.Equal(t, 1, strings.Count(entry.content, "`"+spec.Name+"`"),
						"entrypoint must route to one exact detail target")
					for _, placeholder := range []string{"$ARGUMENTS", "--model <provider/model>", "--variant <value>"} {
						assert.Equal(t, 1, strings.Count(entry.content, placeholder),
							"%s must preserve %s exactly once", entry.name, placeholder)
					}
					if entry.wantNoFuzzy {
						assert.Contains(t, strings.ToLower(entry.content), "do not fuzzy-correct")
					}
				})
			}
		})
	}
}

func mappingContentKeys(files map[string]string) map[string]bool {
	out := make(map[string]bool, len(files))
	for name := range files {
		out[name] = true
	}
	return out
}

func TestOMP002_S5_PlainConfigRemainsByteIdentical(t *testing.T) {
	for _, original := range []string{
		"model: user-model\n",
		"model: user-model\nskills:\n  userDirectory: ./private\n",
		"skills: scalar-owned-by-user\n",
		"skills:\n  customDirectories:\n    - ./user-owned\n",
	} {
		original := original
		t.Run(strings.ReplaceAll(strings.TrimSpace(original), "\n", "_"), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, configFile)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

			_, err := NewWithRoot(dir).Generate(context.Background(), configForOMP())
			require.NoError(t, err)
			after, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, original, string(after))
			info, statErr := os.Stat(path)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		})
	}
}

func TestOMP002_S7_ValidateReportsMissingAndTamperedManagedSets(t *testing.T) {
	dir := generateOMPOnly(t)
	a := NewWithRoot(dir)
	cfg := configForOMP()

	rules, err := a.prepareRuleMappings()
	require.NoError(t, err)
	commands, err := a.prepareCommandMappings(cfg)
	require.NoError(t, err)
	removedRule := removeStableMapping(t, dir, rules)
	removedCommand := removeStableMapping(t, dir, commands)
	require.NoError(t, os.Remove(filepath.Join(dir, ".omp", "skills", "auto-plan", "SKILL.md")))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".omp", "skills", "auto-go", "SKILL.md"),
		[]byte("tampered\n"), 0o600,
	))

	findings, err := a.Validate(context.Background())
	require.NoError(t, err)
	var details []string
	for _, finding := range findings {
		details = append(details, finding.File+" "+finding.Message)
	}
	joined := strings.Join(details, "\n")
	assert.Contains(t, joined, "expected="+itoa(len(rules))+" got="+itoa(len(rules)-1))
	for _, path := range []string{
		removedRule, removedCommand, ".omp/skills/auto-plan/SKILL.md",
	} {
		assert.Contains(t, joined, path)
	}
	assert.Contains(t, joined, "managed path must be a regular file")
	assert.Contains(t, joined, "managed content checksum mismatch")
}

func TestOMP002_S9_DetectRequiresExactSemverAndHonorsCallerDeadline(t *testing.T) {
	skipWithoutPOSIXShellOMP(t)
	writeFakeOMPBinary(t, "omp/not-semver")
	got, err := NewWithRoot(t.TempDir()).Detect(context.Background())
	require.NoError(t, err)
	assert.False(t, got, "an omp/ prefix without a semantic version is not readiness authority")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, cliBinary),
		[]byte("#!/bin/sh\n/bin/sleep 5\nprintf 'omp/17.1.8\\n'\n"), 0o755))
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	got, err = NewWithRoot(t.TempDir()).Detect(ctx)
	require.NoError(t, err)
	assert.False(t, got)
	assert.Less(t, time.Since(started), time.Second, "readiness identity probe must be bounded")
}
