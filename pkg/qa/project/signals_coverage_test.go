package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProjectFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// Guards against the marker-file branch silently widening or narrowing: a bare
// project must stay negative while each declared marker must classify.
func TestDesktopGUISignalsFileMarkers(t *testing.T) {
	t.Parallel()

	empty := t.TempDir()
	assert.False(t, HasDesktopGUISignals(empty))

	for _, marker := range []string{
		"src-tauri/tauri.conf.json",
		"e2e-tests/wdio.conf.mjs",
		"scripts/visual/run-windows-webview2-suite.mjs",
	} {
		dir := t.TempDir()
		writeProjectFile(t, dir, marker, "{}\n")
		assert.True(t, HasDesktopGUISignals(dir), marker)
	}

	// A file that merely lives under a similar tree must not classify.
	other := t.TempDir()
	writeProjectFile(t, other, "scripts/visual/README.md", "notes\n")
	assert.False(t, HasDesktopGUISignals(other))
}

// Guards the package.json fallback: dependency and devDependency hits both
// count, and an unrelated manifest must not classify as desktop GUI.
func TestDesktopGUISignalsPackageDependencies(t *testing.T) {
	t.Parallel()

	runtimeDep := t.TempDir()
	writeProjectFile(t, runtimeDep, "package.json", `{"dependencies":{"electron":"^30.0.0"}}`)
	assert.True(t, HasDesktopGUISignals(runtimeDep))

	devDep := t.TempDir()
	writeProjectFile(t, devDep, "package.json", `{"devDependencies":{"@tauri-apps/api":"^2.0.0"}}`)
	assert.True(t, HasDesktopGUISignals(devDep))

	unrelated := t.TempDir()
	writeProjectFile(t, unrelated, "package.json", `{"dependencies":{"lodash":"^4.0.0"}}`)
	assert.False(t, HasDesktopGUISignals(unrelated))
}

// Guards the script-name heuristic, including its case-insensitivity and the
// substring boundary that keeps unrelated scripts from classifying.
func TestDesktopGUISignalsScriptNames(t *testing.T) {
	t.Parallel()

	for _, script := range []string{"Tauri:Dev", "e2e:appium", "visual:macos", "e2e:linux", "run-WEBDRIVER"} {
		dir := t.TempDir()
		writeProjectFile(t, dir, "package.json", `{"scripts":{"`+script+`":"echo hi"}}`)
		assert.True(t, HasDesktopGUISignals(dir), script)
	}

	neutral := t.TempDir()
	writeProjectFile(t, neutral, "package.json", `{"scripts":{"build":"tsc","test":"vitest"}}`)
	assert.False(t, HasDesktopGUISignals(neutral))
}

// Guards the fail-closed manifest path: unreadable or malformed package.json
// must return false rather than panicking or classifying on partial data.
func TestSignalsRejectUnreadableManifest(t *testing.T) {
	t.Parallel()

	malformed := t.TempDir()
	writeProjectFile(t, malformed, "package.json", `{"dependencies": {"electron"`)
	assert.False(t, HasDesktopGUISignals(malformed))
	assert.False(t, HasBrowserSignals(malformed))

	// package.json as a directory makes ReadFile fail on every platform.
	unreadable := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(unreadable, "package.json"), 0o755))
	assert.False(t, HasDesktopGUISignals(unreadable))
	assert.False(t, HasBrowserSignals(unreadable))
}

// Guards browser classification: config markers and framework dependencies
// classify, while a server-only manifest must not.
func TestBrowserSignalsConfigAndDependencies(t *testing.T) {
	t.Parallel()

	for _, marker := range []string{"playwright.config.ts", "next.config.js", "vite.config.ts", "nuxt.config.ts"} {
		dir := t.TempDir()
		writeProjectFile(t, dir, marker, "export default {}\n")
		assert.True(t, HasBrowserSignals(dir), marker)
	}

	dep := t.TempDir()
	writeProjectFile(t, dep, "package.json", `{"devDependencies":{"@playwright/test":"^1.44.0"}}`)
	assert.True(t, HasBrowserSignals(dep))

	serverOnly := t.TempDir()
	writeProjectFile(t, serverOnly, "package.json", `{"dependencies":{"express":"^4.0.0"}}`)
	assert.False(t, HasBrowserSignals(serverOnly))

	assert.False(t, HasBrowserSignals(t.TempDir()))
}

// Guards the iOS glob branch, which is reached only when none of the fixed
// relative paths exist.
func TestIOSSignalsGlobFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "MyApp.xcworkspace"), 0o755))
	assert.True(t, HasIOSSignals(dir))

	plistOnly := t.TempDir()
	writeProjectFile(t, plistOnly, "Info.plist", "<plist/>\n")
	assert.False(t, HasIOSSignals(plistOnly))
}

// Guards the deliberate Android exclusion of bare gradle files, which would
// otherwise classify every JVM project as an Android app.
func TestAndroidSignalsExcludeBareGradle(t *testing.T) {
	t.Parallel()

	bare := t.TempDir()
	writeProjectFile(t, bare, "build.gradle", "plugins {}\n")
	writeProjectFile(t, bare, "settings.gradle", "rootProject.name = 'x'\n")
	assert.False(t, HasAndroidSignals(bare))

	app := t.TempDir()
	writeProjectFile(t, app, "app/src/main/AndroidManifest.xml", "<manifest/>\n")
	assert.False(t, HasAndroidSignals(app), "nested app manifest is not a declared marker")

	root := t.TempDir()
	writeProjectFile(t, root, "AndroidManifest.xml", "<manifest/>\n")
	assert.True(t, HasAndroidSignals(root))
}
