package companionmanifest

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The unit gate used to be one 54-minute job. It is now five jobs (build,
// test-isolated, test-packages, test-cli shards, and the `test` aggregator), so
// the properties the single job guaranteed by construction have to be pinned
// across jobs instead: the process-heavy run/skip boundary, the split coverage
// profiles, and above all that the internal/cli shards cannot drop a test.

func TestCIWorkflow_UnitGateKeepsProcessHeavyIsolationAndSplitCoverage(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/ci.yaml")
	// The selector has one source, the Makefile. A copy in a job would let the
	// local gate and the CI gate drift apart without any test noticing.
	for _, id := range []string{"test-isolated", "test-packages", "test-cli"} {
		resolver := ciStepRun(t, workflow.Jobs[id], "Resolve process-heavy test selector")
		for _, required := range []string{
			"set -euo pipefail",
			"sed -n 's/^PROCESS_HEAVY_TESTS ?= //p' Makefile",
			`[[ -n "$selector" ]]`,
			"$GITHUB_ENV",
		} {
			if !strings.Contains(resolver, required) {
				t.Fatalf("%s must derive the process-heavy selector from the Makefile; missing %q", id, required)
			}
		}
	}

	isolated := ciStepRun(t, workflow.Jobs["test-isolated"], "Test process-heavy contracts in isolation")
	packages := ciStepRun(t, workflow.Jobs["test-packages"], "Test every package except internal/cli with Coverage")
	shard := ciStepRun(t, workflow.Jobs["test-cli"], "Test internal/cli shard with Coverage")

	// The isolated lane runs exactly the selector; every other race lane skips
	// exactly the selector, so the union is the whole suite with no overlap.
	if !strings.Contains(isolated, `-run "$PROCESS_HEAVY_TESTS"`) || strings.Contains(isolated, "-skip") {
		t.Fatal("isolated lane must run exactly the resolved process-heavy selector")
	}
	for name, lane := range map[string]string{"test-packages": packages, "test-cli shard": shard} {
		if !strings.Contains(lane, `-skip "$PROCESS_HEAVY_TESTS"`) {
			t.Fatalf("%s must skip exactly the resolved process-heavy selector", name)
		}
		if !strings.Contains(lane, "-race") || !strings.Contains(lane, "-coverprofile=") ||
			!strings.Contains(lane, "-timeout=") || strings.Contains(lane, "-tags integration") {
			t.Fatalf("%s must keep the race + coverage gate and stay out of the integration lane:\n%s", name, lane)
		}
	}
	if strings.Contains(packages, "-run ") {
		t.Fatal("test-packages must select by package, never by -run")
	}
	// Isolation exists to take the heavy tests off the shared scheduler, so -p 1
	// is the point, and isolation must not weaken the race gate.
	if !strings.Contains(isolated, "-p 1") || !strings.Contains(isolated, "-race") ||
		!strings.Contains(isolated, "./...") || !strings.Contains(isolated, "-timeout=") {
		t.Fatal("isolated lane must serialise packages across the whole module while keeping the race gate")
	}
	// An empty selector would leave the lane green having run nothing.
	if !strings.Contains(isolated, "TestReleaseHardeningBashContract") {
		t.Fatal("isolated lane lost its inventory guard against an empty selector")
	}
	// internal/cli is the one package test-packages leaves to the shards, and it
	// must be listed exactly once so it cannot fall out of both jobs.
	for _, required := range []string{
		`cli_package="$(go list ./internal/cli)"`, `all_packages="$(go list ./...)"`,
		"cli_matches == 1", `"${packages[@]}"`,
	} {
		if !strings.Contains(packages, required) {
			t.Fatalf("test-packages must cover every package except internal/cli; missing %q", required)
		}
	}
	if !strings.Contains(shard, "./internal/cli") {
		t.Fatal("test-cli shards must run the package test-packages leaves out")
	}

	// Each lane writes its own profile and the aggregator merges all of them.
	merge := ciStepRun(t, workflow.Jobs["test"], "Merge coverage and enforce the threshold")
	for lane, profile := range map[string]string{
		isolated: "coverage-isolated.out", packages: "coverage-packages.out", shard: "coverage-cli.out",
	} {
		if !strings.Contains(lane, "-coverprofile="+profile) || !strings.Contains(merge, profile) {
			t.Fatalf("coverage profile %s must be written by its lane and merged by the aggregator", profile)
		}
	}
	for _, required := range []string{`head -n 1`, "tail -n +2", "go tool cover -func=coverage.out", `"$COVERAGE < $THRESHOLD"`} {
		if !strings.Contains(merge, required) {
			t.Fatalf("coverage merge missing %q", required)
		}
	}
	if !strings.Contains(readReleaseFile(t, ".github/workflows/ci.yaml"), "COVERAGE_THRESHOLD: \"85\"") {
		t.Fatal("coverage threshold is not pinned")
	}
	download := ciUsesStep(t, workflow.Jobs["test"], "actions/download-artifact@")
	if download.With["pattern"] != "ci-coverage-*" {
		t.Fatalf("aggregator must download every ci-coverage-* artifact, got %v", download.With["pattern"])
	}
	for id, name := range map[string]string{
		"test-isolated": "ci-coverage-isolated", "test-packages": "ci-coverage-packages",
		"test-cli": "ci-coverage-cli-${{ strategy.job-index }}",
	} {
		if upload := ciUsesStep(t, workflow.Jobs[id], "actions/upload-artifact@"); upload.With["name"] != name {
			t.Fatalf("%s uploads artifact %v, want %q", id, upload.With["name"], name)
		}
	}
}

func TestCIWorkflow_CLIShardsCoverEveryTestAndTheTestCheckGatesThem(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/ci.yaml")
	cli := workflow.Jobs["test-cli"]

	// The matrix is one axis of consecutive shard numbers, with the total taken
	// from the matrix size. Hand-copied totals, include or exclude entries could
	// make the legs own fewer tests than exist.
	if len(cli.Strategy.Matrix) != 1 {
		t.Fatalf("test-cli matrix must be the single shard axis, got keys %v", mapKeys(cli.Strategy.Matrix))
	}
	shards, _ := cli.Strategy.Matrix["shard"].([]any)
	if len(shards) < 2 {
		t.Fatalf("test-cli must run at least two shards, got %v", cli.Strategy.Matrix["shard"])
	}
	for i, shard := range shards {
		if shard != i+1 {
			t.Fatalf("test-cli shard %d is %v; shards must be numbered 1..N without gaps", i, shard)
		}
	}
	if cli.Strategy.FailFast == nil || *cli.Strategy.FailFast {
		t.Fatal("test-cli must not fail fast: cancelled shards hide failures, and every failing test should surface in one run")
	}
	if cli.Env["SHARD_INDEX"] != "${{ strategy.job-index }}" || cli.Env["SHARD_TOTAL"] != "${{ strategy.job-total }}" {
		t.Fatalf("shard index and total must come from the matrix, got %v", cli.Env)
	}

	lanes := []struct{ name, plan, run string }{
		{"unit", "Plan the unit shard", "Test internal/cli shard with Coverage"},
		{"integration", "Plan the integration shard", "Test internal/cli shard with the integration tag"},
	}
	for _, lane := range lanes {
		plan := ciStepRun(t, cli, lane.plan)
		run := ciStepRun(t, cli, lane.run)
		// The partition is proven in the shard that uses it, and the real
		// `go test` front end must select exactly the planned names.
		for _, required := range []string{
			"lane=" + lane.name, "go run ./scripts/ci/testshard plan",
			`-total "$SHARD_TOTAL" -index "$SHARD_INDEX"`,
			`-names "$RUNNER_TEMP/cli-names-$lane.txt"`, `-manifest "$RUNNER_TEMP/manifest-$lane.txt"`,
			`[[ "$selected" != "$planned" ]]`, "exit 1",
		} {
			if !strings.Contains(plan, required) {
				t.Fatalf("%s step missing %q", lane.plan, required)
			}
		}
		if !strings.Contains(run, `-run "$(<"$RUNNER_TEMP/cli-run-`+lane.name+`.expr")"`) || !strings.Contains(run, "-timeout=") {
			t.Fatalf("%s must run exactly its planned expression under a timeout:\n%s", lane.run, run)
		}
		if ciStepIndex(t, cli, lane.plan) >= ciStepIndex(t, cli, lane.run) {
			t.Fatalf("%s must precede %s", lane.plan, lane.run)
		}
	}
	unitPlan := ciStepRun(t, cli, "Plan the unit shard")
	integrationPlan := ciStepRun(t, cli, "Plan the integration shard")
	integrationRun := ciStepRun(t, cli, "Test internal/cli shard with the integration tag")
	// Each plan must list with the same flags its lane runs with.
	if !strings.Contains(unitPlan, `-skip "$PROCESS_HEAVY_TESTS"`) || !strings.Contains(unitPlan, "-race") {
		t.Fatal("the unit shard plan must apply the process-heavy skip and list under -race")
	}
	if strings.Count(integrationPlan, "-tags integration") != 2 || strings.Contains(integrationPlan, "-skip") {
		t.Fatal("the integration shard plan must list with the integration tag and, like the lane, skip nothing")
	}
	if !strings.Contains(integrationRun, "-tags integration") || strings.Contains(integrationRun, "-race") ||
		strings.Contains(integrationRun, "-coverprofile") {
		t.Fatal("the integration shard must stay isolated from race coverage instrumentation")
	}

	// `test` is the one check that means "the unit gate passed".
	test := workflow.Jobs["test"]
	needs := append([]string{}, test.Needs...)
	sort.Strings(needs)
	if strings.Join(needs, ",") != "build,test-cli,test-isolated,test-packages" {
		t.Fatalf("test must need every unit job, got %v", test.Needs)
	}
	// Without always-style evaluation a failed need would leave this check
	// skipped, and a skipped check does not block a merge.
	if !strings.Contains(test.If, "!cancelled()") {
		t.Fatalf("test must still run, and fail, when a need failed; if = %q", test.If)
	}
	if len(test.Steps) == 0 {
		t.Fatal("test has no steps")
	}
	if first := test.Steps[0]; first.Name != "Require every unit job to succeed" ||
		first.Env["NEEDS"] != "${{ toJSON(needs) }}" || !strings.Contains(first.Run, `.result == "success"`) {
		t.Fatalf("test must first fail on any non-success need, got %+v", first)
	}
	verify := ciStepRun(t, test, "Verify internal/cli shards cover every test")
	if !strings.Contains(verify, "go run ./scripts/ci/testshard verify -lanes unit,integration") ||
		!strings.Contains(verify, "ci-coverage-cli-*/manifest-*.txt") {
		t.Fatalf("test must verify the manifests of both shard lanes:\n%s", verify)
	}
	if ciStepIndex(t, test, "Verify internal/cli shards cover every test") >=
		ciStepIndex(t, test, "Merge coverage and enforce the threshold") {
		t.Fatal("shard coverage must be proven before coverage is merged")
	}
	if e2e := workflow.Jobs["e2e"]; strings.Join(e2e.Needs, ",") != "test" {
		t.Fatalf("e2e must wait for the aggregated unit gate, got %v", e2e.Needs)
	}
}

// A job whose timeout is shorter than the sum of its go test timeouts is killed
// by the runner before the test binary can print its goroutine dump, which
// turns a diagnosable hang into a silent cancel. Pin the relationship, not the
// minutes.
func TestCIWorkflow_JobBudgetsOutliveTheirGoTestTimeouts(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/ci.yaml")
	timeout := regexp.MustCompile(`-timeout=(\d+)m`)
	const setupAndTeardownMinutes = 5
	for _, id := range []string{"test-isolated", "test-packages", "test-cli", "lineage-integration", "integration-packages"} {
		job := workflow.Jobs[id]
		total, binaries := 0, 0
		for _, step := range job.Steps {
			for _, match := range timeout.FindAllStringSubmatch(step.Run, -1) {
				minutes, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatalf("%s: unreadable timeout %q", id, match[0])
				}
				total += minutes
				binaries++
			}
		}
		if binaries == 0 {
			t.Fatalf("%s runs no go test with -timeout", id)
		}
		if job.TimeoutMinutes < total+setupAndTeardownMinutes {
			t.Fatalf("%s timeout-minutes = %d cannot hold its go test timeouts (%dm in %d runs) plus %dm of setup",
				id, job.TimeoutMinutes, total, binaries, setupAndTeardownMinutes)
		}
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
