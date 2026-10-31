package gates

// EscalationLeanSurfaceExceeded names a lean SPEC whose change set touched
// production paths outside the surface the SPEC declared.
const EscalationLeanSurfaceExceeded = "lean_spec_change_outside_declared_surface"

// EscalateOutsideSurface raises a risk record to feature when a lean SPEC's
// change set left its declared surface. A lean SPEC was authored and reviewed
// for that surface only, so work beyond it needs the full SPEC set.
func (r ChangeRisk) EscalateOutsideSurface(outside []string) ChangeRisk {
	if len(outside) == 0 {
		return r
	}
	r.Reasons = append(append([]string(nil), r.Reasons...), EscalationLeanSurfaceExceeded)
	if kindRank(KindFeature) > kindRank(r.EffectiveClass) {
		r.EffectiveClass = KindFeature
	}
	r.Tier = RiskHigh
	r.Decision = DecisionEscalate
	return r
}

// PathsOutsideSurface returns the production paths of changed that surface
// does not cover. A surface entry covers itself and everything beneath it.
func PathsOutsideSurface(surface, changed []string) []string {
	return ChangeContract{Surface: surface}.PathsOutsideSurface(changed)
}
