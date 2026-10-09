package healthband_test

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// RR-7, Gradle: an included build runs while Gradle configures the build or
// an IDE syncs it, so the directory that a settings file's includeBuild
// names is denied, whether or not the base tracks anything there. A call
// that does not name its directory with a plain literal denies the settings
// file's own directory tree.
func TestPatchPolicy_GradleIncludedBuilds_DenyTheirDirectories(t *testing.T) {
	t.Parallel()
	kt := func(line string) string { return line + "\n" }
	repo := newPolicyRepo(t, []baseFile{
		{path: "settings.gradle.kts", content: "pluginManagement {\n    includeBuild(\"gradle/plugins\")\n}\n" +
			"includeBuild(file(\"tools/conventions\")) { name = \"conventions\" }\n" +
			"includeBuild(\"build-tools/untracked\")\nincludeBuild(\"pl\u00fcgins\")\n" +
			"// includeBuild is documented in docs/build.md\ninclude(\":app\")\n"},
		{path: "app/src/main/kotlin/App.kt", content: kt("fun main() {}")},
		{path: "gradle/plugins/settings.gradle.kts", content: kt(`includeBuild("../../platform")`)},
		{path: "gradle/plugins/src/main/kotlin/Conv.kt", content: kt("class Conv")},
		{path: "sub/settings.gradle", content: "includeBuild '../shared-build'\nincludeBuild('nested') {\n}\n"},
		{path: "dyn/settings.gradle.kts", content: kt(`includeBuild("$rootDir/x")`)},
		{path: "prose/settings.gradle", content: kt("// includeBuild for later")},
		{path: "mref/settings.gradle.kts", content: kt(`listOf("a").forEach(::includeBuild)`)},
	})
	newKt := func(path string) string { return newFile(path, "100644", "class X") }
	assertPathVerdicts(t, repo, map[string]pathCase{
		"plugin build":                       {newKt("gradle/plugins/src/main/kotlin/New.kt"), denied},
		"edit in the plugin build":           {appendLine("gradle/plugins/src/main/kotlin/Conv.kt", "class Conv", "// x"), denied},
		"build named through file()":         {newKt("tools/conventions/src/X.kt"), denied},
		"build an included build includes":   {newKt("platform/src/P.kt"), denied},
		"Groovy command call":                {newKt("shared-build/src/S.kt"), denied},
		"Groovy call with a closure":         {newFile("sub/nested/src/N.java", "100644", "class N {}"), denied},
		"untracked build, another case":      {newKt("Build-Tools/Untracked/src/U.kt"), denied},
		"untracked build, NFD spelling":      {newKt("plu\u0308gins/src/U.kt"), denied},
		"tracked build, another case":        {newKt("Gradle/plugins/src/C.kt"), healthband.PatchCodeCaseCollision},
		"templated name denies its tree":     {newKt("dyn/src/D.kt"), denied},
		"Groovy prose fails closed":          {newKt("prose/src/P.kt"), denied},
		"Kotlin method reference":            {newKt("mref/src/M.kt"), denied},
		"project of the build, Kotlin prose": {appendLine("app/src/main/kotlin/App.kt", "fun main() {}", "// x"), ""},
	})
}
