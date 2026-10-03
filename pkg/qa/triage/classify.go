package triage

// Classify assigns exactly one class to a failed journey.
//
// Placeholder until the rule set lands: every journey is unknown, which makes
// the loop stop instead of fixing anything.
func Classify(in Input) Verdict {
	return Verdict{JourneyID: in.JourneyID, Class: ClassUnknown, Signal: "no_rule_matched"}
}

// InputFromManifest builds an Input from a QAMESH evidence manifest written
// by `auto qa run`, reading the bounded failure excerpt from its artifacts.
//
// Placeholder until the rule set lands.
func InputFromManifest(projectDir, manifestPath string) (Input, error) {
	return Input{ProjectDir: projectDir}, nil
}
