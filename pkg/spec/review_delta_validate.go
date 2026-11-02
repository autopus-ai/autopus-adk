package spec

import (
	"fmt"
	"strings"
)

// DeltaManifestCheck is what ValidateDeltaManifest proves a delta prompt
// accounts for: the manifest it declared, every required source of the round
// with its current digests, the baseline, the plan, and the prompt.
type DeltaManifestCheck struct {
	Manifest []DeltaSource
	Required []DeltaSource
	Baseline *ReviewBaseline
	Plan     DeltaPlan
	Prompt   string
}

// ValidateDeltaManifest proves the delta prompt carries everything a
// reviewer with no memory needs (REQ-RR-DELTA-05): each required source is
// declared with its current source and delivered digests; a linked source is
// a SPEC document byte-identical to the baseline that the baseline round
// delivered complete; a changed source carries its diff; every unit of a
// SPEC document the request must carry is in it; and an included source's
// full delivered text is in it.
func ValidateDeltaManifest(c DeltaManifestCheck) error {
	declared := make(map[string]DeltaSource, len(c.Manifest))
	for _, source := range c.Manifest {
		declared[source.Name] = source
	}
	for _, required := range c.Required {
		source, ok := declared[required.Name]
		switch {
		case !ok:
			return fmt.Errorf("delta manifest omits %s", required.Name)
		case source.SourceSHA256 != required.SourceSHA256 || source.PromptSHA256 != required.PromptSHA256:
			return fmt.Errorf("delta manifest digest of %s is stale", required.Name)
		}
		if err := c.validateDelivery(source); err != nil {
			return err
		}
	}
	return nil
}

func (c DeltaManifestCheck) validateDelivery(source DeltaSource) error {
	_, document := c.Plan.current[source.Name]
	switch source.Delivery {
	case DeltaDeliveryLinked:
		baseline, ok := c.Baseline.Documents[source.Name]
		switch {
		case !document:
			return fmt.Errorf("%s is linked, but only an unchanged SPEC document may be", source.Name)
		case !ok || baseline.SourceSHA256 != source.SourceSHA256 || baseline.PromptSHA256 != source.PromptSHA256:
			return fmt.Errorf("%s is linked but differs from the baseline", source.Name)
		case c.Baseline.Delivered[source.Name] != BaselineDeliveredComplete:
			return fmt.Errorf("%s is linked but the baseline round did not deliver it complete", source.Name)
		}
		return c.unitsCarried(source.Name)
	case DeltaDeliveryChanged:
		if !document || !strings.Contains(c.Prompt, "##### "+source.Name+"\n\n```diff\n") {
			return fmt.Errorf("%s is changed but its diff is missing", source.Name)
		}
		return c.unitsCarried(source.Name)
	case DeltaDeliveryIncluded:
		if document || source.Text == "" || SHA256Hex([]byte(source.Text)) != source.PromptSHA256 || !strings.Contains(c.Prompt, source.Text) {
			return fmt.Errorf("%s is included but its text is missing", source.Name)
		}
		return nil
	}
	return fmt.Errorf("%s has unknown delivery %q", source.Name, source.Delivery)
}

// unitsCarried proves the prompt carries, in full, every unit of a SPEC
// document the request must carry, and for spec.md the traceability rows.
func (c DeltaManifestCheck) unitsCarried(name string) error {
	layout := c.Plan.current[name]
	for _, start := range c.Plan.requiredUnits(name) {
		if !strings.Contains(c.Prompt, layout.unitBody(start)) {
			return fmt.Errorf("%s: the request does not carry %s", name, layout.unitName(start))
		}
	}
	if name != "spec.md" {
		return nil
	}
	for _, row := range c.Plan.traceabilityRows() {
		if !strings.Contains(c.Prompt, row) {
			return fmt.Errorf("spec.md: the request does not carry the traceability row %q", row)
		}
	}
	return nil
}
