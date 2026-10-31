package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// loadLeanGatesSpec returns the SPEC when it declares the lean profile, so the
// gate decision can hold the real change set to the class and surface the
// lean SPEC was authored and reviewed for. Any load failure means no lean
// claim, which leaves the ordinary derived-class decision in place.
func loadLeanGatesSpec(specDir string) *spec.SpecDocument {
	doc, err := spec.Load(specDir)
	if err != nil || !doc.IsLean() {
		return nil
	}
	return doc
}

// assessLeanGatesChange escalates a lean SPEC's change set that left its
// declared surface. Escalation makes spec_authoring and risk_first_probe
// required, which is what sends the work back to a full SPEC set.
func assessLeanGatesChange(lean *spec.SpecDocument, risk gates.ChangeRisk, paths []string) (gates.ChangeRisk, []string) {
	if lean == nil {
		return risk, nil
	}
	outside := gates.PathsOutsideSurface(lean.Surface, paths)
	return risk.EscalateOutsideSurface(outside), outside
}

func printLeanGatesEscalation(w io.Writer, lean *spec.SpecDocument, risk gates.ChangeRisk, outside []string) {
	if lean == nil || !risk.Escalated() {
		return
	}
	fmt.Fprintf(w, "lean SPEC 승격 필요: 실제 변경이 %s로 판정됨 (%s)\n", risk.EffectiveClass, strings.Join(risk.Reasons, ", "))
	if len(outside) > 0 {
		fmt.Fprintf(w, "선언한 surface 밖의 경로: %s\n", strings.Join(outside, ", "))
	}
	fmt.Fprintln(w, "spec.md의 profile: lean을 지우고 full SPEC set으로 다시 작성한 뒤 리뷰하세요")
}
