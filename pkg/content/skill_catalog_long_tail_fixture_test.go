package content

// init registers the long-tail catalog skills these tests name for
// IsInstalledSkill, in the shape v0.50.134 shipped them: registered on every
// platform, in an opt-in bundle, and not on the default surface.
func init() {
	longTailFixtureSkills = map[string]CatalogSkill{}
	for name, bundle := range map[string]string{
		"metrics": "product", "docker": "ops", "ddd": "quality", "ast-refactoring": "quality",
	} {
		longTailFixtureSkills[name] = CatalogSkill{
			Name:           name,
			CompileTargets: []string{"claude", "codex", "gemini", "opencode", "omp"},
			Visibility:     SkillVisibilityShared,
			Bundles:        []string{bundle},
		}
	}
}
