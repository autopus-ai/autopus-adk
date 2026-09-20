package taskroute

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func pointer[T any](value T) *T { return &value }
func small() Facts {
	return Facts{Version: 1, Kind: "bugfix", Paths: []string{"pkg/widget/update.go"}, ScopeComplete: pointer(true), RequirementsClear: pointer(true), AcceptanceKnown: pointer(true), EstimatedChangedLines: pointer(20), Risk: "low"}
}
func TestInlineRequiresCompletePositiveEvidence(t *testing.T) {
	f := small()
	d, err := Decide(f)
	if err != nil || d.Route != "inline" || d.ModelPolicy != "inherit" || !d.Advisory {
		t.Fatalf("%+v %v", d, err)
	}
	for _, mutate := range []func(*Facts){func(f *Facts) { f.ScopeComplete = nil }, func(f *Facts) { f.RequirementsClear = nil }, func(f *Facts) { f.AcceptanceKnown = nil }, func(f *Facts) { f.EstimatedChangedLines = nil }, func(f *Facts) { f.Risk = "" }, func(f *Facts) { f.Paths = nil }} {
		f := small()
		mutate(&f)
		d, err := Decide(f)
		if err != nil || d.Route != "guided" {
			t.Fatalf("%+v %v", d, err)
		}
	}
}
func TestRiskAndComplexityCannotBeBypassed(t *testing.T) {
	for _, mutate := range []func(*Facts){func(f *Facts) { f.Risk = "high" }, func(f *Facts) { f.Paths = []string{"pkg/auth/check.go"} }, func(f *Facts) { f.Paths = []string{"pkg/payment/refund.go"} }, func(f *Facts) { f.Paths = []string{"pkg/a/a.go", "pkg/b/b.go"} }, func(f *Facts) { f.FailedAttempts = 2 }, func(f *Facts) { f.RequirementsClear = pointer(false) }, func(f *Facts) { f.EstimatedChangedLines = pointer(301) }} {
		f := small()
		mutate(&f)
		f.Requested = "inline"
		d, err := Decide(f)
		if err != nil || d.Route != "planned" || !slices.Contains(d.Reasons, "requested_route_below_required") {
			t.Fatalf("%+v %v", d, err)
		}
	}
	f := small()
	f.Paths = []string{"pkg/auth/one.go"}
	d, _ := Decide(f)
	for _, step := range []string{"security_review", "integration_acceptance", "scoped_verification", "honest_report"} {
		if !slices.Contains(d.RequiredSteps, step) {
			t.Fatal(d)
		}
	}
}
func TestParallelRequiresDisjointBoundedWorkers(t *testing.T) {
	f := small()
	f.Requested = "planned"
	f.Paths = []string{"pkg/a", "pkg/ab", "pkg/b"}
	f.NativeParallelAvailable = true
	f.Workers = []Worker{{ID: "a", Independent: true, OwnedPaths: []string{"pkg/a"}}, {ID: "b", Independent: true, OwnedPaths: []string{"pkg/ab"}}}
	d, err := Decide(f)
	if err != nil || d.Execution != "parallel" {
		t.Fatalf("%+v %v", d, err)
	}
	f.Workers[1].OwnedPaths = []string{"pkg/a/child.go"}
	d, _ = Decide(f)
	if d.Execution != "serial" || !slices.Contains(d.Reasons, "ownership_overlap") {
		t.Fatal(d)
	}
	f.Workers[1].OwnedPaths = []string{"pkg/b"}
	f.Solo = true
	d, _ = Decide(f)
	if d.Execution != "serial" {
		t.Fatal(d)
	}
}
func TestStrictDecodeAndUnsafePaths(t *testing.T) {
	for _, raw := range []string{`{"version":1,"kind":"docs","kind":"bugfix"}`, `{"version":1,"kind":"docs","extra":1}`, `{"version":1,"kind":"docs"} {}`, `null`} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatal(raw)
		}
	}
	for _, path := range []string{".", "*", "../x", "/abs", "a/../b", "C:/x", "a\\b", "a/**"} {
		f := small()
		f.Paths = []string{path}
		if _, err := Decide(f); err == nil {
			t.Fatal(path)
		}
	}
	f := small()
	f.Workers = []Worker{{ID: "a", Independent: true, OwnedPaths: []string{"pkg/a"}}, {ID: "a", OwnedPaths: []string{"pkg/b"}}}
	if _, err := Decide(f); err == nil {
		t.Fatal("duplicate worker")
	}
}

func TestWorkerIndependenceMustBeExplicit(t *testing.T) {
	f := small()
	f.Requested = "planned"
	f.Paths = []string{"pkg/a", "pkg/ab", "pkg/b"}
	f.NativeParallelAvailable = true
	f.Workers = []Worker{{ID: "a", OwnedPaths: []string{"pkg/a"}}, {ID: "b", OwnedPaths: []string{"pkg/b"}, Independent: true}}
	d, err := Decide(f)
	if err != nil || d.Execution != "serial" || !slices.Contains(d.Reasons, "independence_unverified") {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestPolicyBoundariesAndMediumRisk(t *testing.T) {
	for _, tc := range []struct {
		lines, files, failures int
		risk, kind, want       string
	}{
		{80, 2, 0, "low", "bugfix", "inline"}, {81, 2, 0, "low", "bugfix", "guided"},
		{300, 5, 0, "low", "feature", "guided"}, {301, 5, 0, "low", "feature", "planned"},
		{20, 6, 0, "low", "bugfix", "planned"}, {20, 1, 1, "low", "bugfix", "guided"},
		{20, 1, 0, "medium", "docs", "guided"}, {20, 1, 0, "critical", "bugfix", "planned"},
	} {
		f := small()
		f.Paths = nil
		for i := 0; i < tc.files; i++ {
			f.Paths = append(f.Paths, fmt.Sprintf("pkg/widget/f%d.go", i))
		}
		f.EstimatedChangedLines = &tc.lines
		f.FailedAttempts = tc.failures
		f.Risk = tc.risk
		f.Kind = tc.kind
		d, err := Decide(f)
		if err != nil || d.Route != tc.want {
			t.Fatalf("%+v: %+v %v", tc, d, err)
		}
	}
	f := small()
	f.AcceptanceKnown = pointer(false)
	d, _ := Decide(f)
	if d.Route != "guided" || !slices.Contains(d.RequiredSteps, "define_acceptance") {
		t.Fatal(d)
	}
}
func TestDecodeNestedDuplicatesLimitsAndValidUnknown(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"kind":"bugfix","workers":[{"id":"a","id":"b","owned_paths":["pkg/a"]}]}`,
		strings.Repeat(" ", MaxFactsBytes+1),
		`{"version":1,"kind":"bugfix","failed_attempts":11}`,
		`{"version":1,"kind":"bugfix","estimated_changed_lines":-1}`,
	} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid facts accepted")
		}
	}
	f, err := Decode(strings.NewReader(`{"version":1,"kind":"bugfix"}`))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decide(f)
	if err != nil || d.Route != "guided" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestParallelRequiresParentScopeAndPositiveFacts(t *testing.T) {
	for _, mutate := range []func(*Facts){
		func(f *Facts) { f.Paths = []string{"pkg/a/x.go", "pkg/b"} },
		func(f *Facts) { f.ScopeComplete = nil }, func(f *Facts) { f.RequirementsClear = nil }, func(f *Facts) { f.AcceptanceKnown = pointer(false) },
	} {
		f := small()
		f.Requested = "planned"
		f.Paths = []string{"pkg/a", "pkg/b"}
		f.NativeParallelAvailable = true
		f.Workers = []Worker{{ID: "a", OwnedPaths: []string{"pkg/a"}, Independent: true}, {ID: "b", OwnedPaths: []string{"pkg/b"}, Independent: true}}
		mutate(&f)
		d, err := Decide(f)
		if err != nil || d.Execution != "serial" || !slices.Contains(d.Reasons, "worker_scope_unverified") {
			t.Fatalf("%+v %v", d, err)
		}
	}
}

func TestAggregateOwnershipBoundIncludesSolo(t *testing.T) {
	f := small()
	f.Solo = true
	for i := 0; i < 32; i++ {
		w := Worker{ID: fmt.Sprintf("worker%d", i)}
		count := 8
		if i == 31 {
			count = 9
		}
		for j := 0; j < count; j++ {
			w.OwnedPaths = append(w.OwnedPaths, fmt.Sprintf("pkg/p%d/f%d.go", i, j))
		}
		f.Workers = append(f.Workers, w)
	}
	if _, err := Decide(f); err == nil {
		t.Fatal("257 aggregate ownership paths accepted")
	}
	f.Workers[31].OwnedPaths = f.Workers[31].OwnedPaths[:8]
	if _, err := Decide(f); err != nil {
		t.Fatalf("256 bounded paths rejected: %v", err)
	}
}
func TestPlannedReviewIsMandatoryAndSecurityAdditive(t *testing.T) {
	for _, mutate := range []func(*Facts){func(f *Facts) { f.Requested = "planned" }, func(f *Facts) { f.Paths = []string{"pkg/a/a.go", "pkg/b/b.go"} }, func(f *Facts) { f.RequirementsClear = pointer(false) }, func(f *Facts) { f.Risk = "high" }} {
		f := small()
		mutate(&f)
		d, err := Decide(f)
		if err != nil || d.Route != "planned" || !slices.Contains(d.RequiredSteps, "review_changes") {
			t.Fatalf("%+v %v", d, err)
		}
		if f.Risk == "high" && !slices.Contains(d.RequiredSteps, "security_review") {
			t.Fatal("lost security review")
		}
	}
	for _, risk := range []string{"low", "medium"} {
		f := small()
		f.Risk = risk
		d, _ := Decide(f)
		if slices.Contains(d.RequiredSteps, "review_changes") {
			t.Fatal("review default outside planned")
		}
	}
}

func TestDirectoryGrantsIdentifyDistinctDomains(t *testing.T) {
	f := small()
	f.Kind = "feature"
	f.Risk = "medium"
	f.EstimatedChangedLines = pointer(180)
	f.Paths = []string{"pkg/parser", "pkg/format"}
	f.NativeParallelAvailable = true
	f.Workers = []Worker{{ID: "parser", OwnedPaths: []string{"pkg/parser"}, Independent: true}, {ID: "format", OwnedPaths: []string{"pkg/format"}, Independent: true}}
	d, err := Decide(f)
	if err != nil || d.Route != "planned" || d.Execution != "parallel" || !slices.Contains(d.Reasons, "multiple_domains") {
		t.Fatalf("%+v %v", d, err)
	}
	f.Paths = []string{"pkg/parser", "pkg/parser/format.go"}
	f.Workers = nil
	d, err = Decide(f)
	if err != nil || d.Route != "guided" {
		t.Fatalf("same domain %+v %v", d, err)
	}
	f.Paths = []string{"src/a.go", "src/b.go"}
	d, err = Decide(f)
	if err != nil || d.Route != "guided" {
		t.Fatalf("top-level files became domains %+v %v", d, err)
	}
}
