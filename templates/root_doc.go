package templates

import (
	"fmt"
	"strings"
)

// The AGENTS.md marker section is authored by whichever adapter owns the root
// document: codex owns it unless opencode is installed, in which case opencode
// replaces the whole marker (see codexOwnsRootDoc). Both adapters therefore
// render the same policy, and these fragments are the single source for the
// parts that must not differ between them. Anything a platform is entitled to
// state differently — installed paths, execution model, native routing — stays
// in that adapter's own template.

// RootDocInstalledComponents returns native discovery roots. Detailed generated
// file inventories remain in each platform's ownership manifest.
func RootDocInstalledComponents() string {
	return mustReadSharedFragment("shared/root-doc-installed.md.tmpl")
}

// RootDocPolicy returns the platform-independent H2 policy sections of the
// AGENTS.md marker section: language policy, branding, and document storage.
func RootDocPolicy() string {
	return mustReadSharedFragment("shared/root-doc-policy.md.tmpl")
}

// RootDocGuidelines returns the platform-independent H3 subsections that sit
// under the marker section's "Core Guidelines" heading.
func RootDocGuidelines() string {
	return mustReadSharedFragment("shared/root-doc-guidelines.md.tmpl")
}

// mustReadSharedFragment panics on a missing fragment: the file is embedded at
// build time, so absence is a build defect rather than a runtime condition.
func mustReadSharedFragment(name string) string {
	data, err := FS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("embedded template %s: %v", name, err))
	}
	return strings.TrimRight(string(data), "\n")
}
