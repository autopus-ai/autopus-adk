package tui

// Exported aliases for testing unexported functions.
var (
	TestBuildStepList         = buildStepList
	TestLangOptions           = langOptions
	TestDefaultResult         = defaultResult
	TestBuildLangStep         = buildLangStep
	TestBuildQualityStep      = buildQualityStep
	TestQualityOptions        = qualityOptions
	TestBuildReviewGateStep   = buildReviewGateStep
	TestReviewGateDescription = reviewGateDescription
	TestBuildMethodologyStep  = buildMethodologyStep
	TestBuildProfileStep      = buildProfileStep
	TestBuildReaStepFor       = buildReaStepFor
	TestReaStepTextFor        = reaStepTextFor
	TestWizardStreams         = wizardStreams
	TestWizardStepKinds       = wizardStepKinds
)
