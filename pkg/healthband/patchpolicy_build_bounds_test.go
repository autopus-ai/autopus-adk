package healthband_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// RR-7 bounds: band reads at most PatchManifestMaxFiles manifests and
// PatchManifestMaxTotalBytes in all, shallowest first, and every manifest
// past a bound denies its directory tree.
func TestPatchPolicy_ManifestCountBound_DeniesTheUnreadDirectories(t *testing.T) {
	t.Parallel()
	// Manifests only, so the fixture stays small; each case adds a source.
	var files []baseFile
	for i := 0; i <= healthband.PatchManifestMaxFiles; i++ {
		files = append(files, baseFile{path: fmt.Sprintf("c%04d/Cargo.toml", i), content: "[package]\nname = \"c\"\n"})
	}
	// Deeper than every other manifest, so read last although it sorts first.
	files = append(files, baseFile{path: "a/deep/Cargo.toml", content: "[package]\nname = \"deep\"\n"})
	source := func(dir string) string { return newFile(dir+"/src/new.rs", "100644", libLine) }
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"first manifest read":    {source("c0000"), ""},
		"last manifest read":     {source(fmt.Sprintf("c%04d", healthband.PatchManifestMaxFiles-1)), ""},
		"past the count":         {source(fmt.Sprintf("c%04d", healthband.PatchManifestMaxFiles)), denied},
		"deeper manifests last":  {source("a/deep"), denied},
		"outside every manifest": {newFile("tools/gen.go", "100644", "package tools"), ""},
	})
}

func TestPatchPolicy_ManifestByteBound_DeniesTheUnreadDirectories(t *testing.T) {
	t.Parallel()
	count := healthband.PatchManifestMaxTotalBytes/healthband.PatchManifestMaxBytes + 1
	var files []baseFile
	for i := 0; i < count; i++ {
		head := fmt.Sprintf("[package]\nname = \"c%02d\"\n", i)
		pad := "#" + strings.Repeat("x", healthband.PatchManifestMaxBytes-len(head)-2) + "\n"
		files = append(files, crateFiles(fmt.Sprintf("c%02d", i), head+pad)...)
	}
	require.Len(t, files[0].content, healthband.PatchManifestMaxBytes, "each manifest is at the per-file bound")
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"within the total": {editLib(fmt.Sprintf("c%02d", count-2)), ""},
		"past the total":   {editLib(fmt.Sprintf("c%02d", count-1)), denied},
	})
}

// Item 5 already keeps every patch away from the manifests themselves:
// .toml, .gradle, and .kts are not allowed extensions.
func TestPatchPolicy_BuildManifests_AreNeverPatched(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, []baseFile{
		{path: "Cargo.toml", content: "[package]\nname = \"x\"\n"},
		{path: "settings.gradle.kts", content: "include(\":app\")\n"},
	})
	assertPathVerdicts(t, repo, map[string]pathCase{
		"edit Cargo.toml":          {appendLine("Cargo.toml", "[package]", "build = \"x.rs\""), denied},
		"new crate manifest":       {newFile("macros/Cargo.toml", "100644", "[lib]", "proc-macro = true"), denied},
		"edit settings.gradle.kts": {appendLine("settings.gradle.kts", "include(\":app\")", "includeBuild(\"x\")"), denied},
		"new settings.gradle":      {newFile("sub/settings.gradle", "100644", "includeBuild 'x'"), denied},
	})
}

// The manifest read is one git cat-file --batch over the base listing's
// blob OIDs; output git never prints, or a failure, refuses the diff
// patch_invalid, and a base without manifests reads no blob.
func TestPatchPolicy_ManifestRead_GitFaultsRefusePatchInvalid(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, crates(map[string]string{"app": "[package]\nname = \"app\"\n"}))
	isRead := func(args []string) bool { return args[0] == "cat-file" }
	for name, rewrite := range map[string]func(out []byte) []byte{
		"garbage":       func([]byte) []byte { return []byte("garbage\n") },
		"truncated":     func(out []byte) []byte { return out[:len(out)-2] },
		"trailing data": func(out []byte) []byte { return append(out, "x\n"...) },
		"other size":    func(out []byte) []byte { return bytes.Replace(out, []byte(" blob "), []byte(" blob 1"), 1) },
		"other type":    func(out []byte) []byte { return bytes.Replace(out, []byte(" blob "), []byte(" tree "), 1) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			git := &recordingGit{home: repo.home, rewrite: func(args []string, out []byte) []byte {
				if isRead(args) {
					return rewrite(out)
				}
				return out
			}}
			assert.Equal(t, healthband.PatchCodeInvalid, evaluate(t, repo, healthband.PatchPolicy{Git: git}, replyWith(editLib("app"))).Code)
			assert.True(t, git.invoked("cat-file", "--batch"))
		})
	}
	plain := &recordingGit{home: repo.home}
	failing := healthband.GitRunnerFunc(func(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
		if isRead(args) {
			return nil, errors.New("git cat-file: exit status 128")
		}
		return plain.Run(ctx, dir, stdin, args...)
	})
	assert.Equal(t, healthband.PatchCodeInvalid, evaluate(t, repo, healthband.PatchPolicy{Git: failing}, replyWith(editLib("app"))).Code)

	base := newPolicyRepo(t, policyBase)
	git := &recordingGit{home: base.home}
	require.True(t, evaluate(t, base, healthband.PatchPolicy{Git: git}, replyWith(modifyFoo("// x"))).Accepted())
	assert.False(t, git.invoked("cat-file"), "a base without manifests reads no blob")
}

// A manifest path is resolved against the manifest's directory: one that
// climbs out of the repository is outside every patch, and an absolute one
// counts inside the user's checkout only.
func TestPatchPolicy_AbsoluteBuildPath_CountsInsideTheCheckout(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := newPolicyRepoAt(t, dir, crates(map[string]string{
		"app": "[package]\nname = \"app\"\n\n[build-dependencies]\ngen = { path = \"" + dir + "/gen\" }\n" +
			"elsewhere = { path = \"/elsewhere/gen\" }\nup = { path = \"../../up\" }\n",
		"gen": "[package]\nname = \"gen\"\n",
	}))
	assertPathVerdicts(t, repo, map[string]pathCase{
		"absolute path inside the checkout": {editLib("gen"), denied},
		"the crate that names it":           {editLib("app"), ""},
	})
}
