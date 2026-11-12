package companionmanifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciStabilityStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

type ciStabilityStrategy struct {
	FailFast *bool          `yaml:"fail-fast"`
	Matrix   map[string]any `yaml:"matrix"`
}

type ciStabilityJob struct {
	Name           string              `yaml:"name"`
	RunsOn         string              `yaml:"runs-on"`
	TimeoutMinutes int                 `yaml:"timeout-minutes"`
	Needs          []string            `yaml:"needs"`
	If             string              `yaml:"if"`
	Strategy       ciStabilityStrategy `yaml:"strategy"`
	Env            map[string]string   `yaml:"env"`
	Steps          []ciStabilityStep   `yaml:"steps"`
}

type ciStabilityWorkflow struct {
	Jobs map[string]ciStabilityJob `yaml:"jobs"`
}

// TestCIWorkflow_StableChecksHaveBoundedTimeouts는 이름 그대로 "예산이 유한한가"를
// 본다. 예전에는 각 job의 분 단위 값을 그대로 베껴 뒀는데, 그러면 정당한 예산
// 조정마다 테스트가 깨지고 사람은 값을 다시 베끼는 것으로 대응한다 - 검증이 아니라
// 필사(筆寫)다. 지금은 상한만 강제한다: 예산은 존재해야 하고, 폭주 job을 잡을 만큼
// 낮아야 한다.
//
// 반대로 action SHA와 툴 버전은 계속 정확히 고정한다. 그건 드리프트가 아니라
// 공급망 고정이며, 값이 바뀌었다는 사실 자체가 리뷰 대상이다.
func TestCIWorkflow_StableChecksHaveBoundedTimeouts(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/ci.yaml")
	// 상한은 "이 job이 이보다 오래 걸리면 무언가 잘못됐다"는 뜻이다.
	ceilings := map[string]int{
		"build": 30, "test-isolated": 30, "test-packages": 60, "test-cli": 60,
		"test": 20, "lineage-integration": 60, "integration-packages": 30, "e2e": 20, "lint": 20,
		"static-contracts": 20, "macos-runtime": 30,
		"omp-native-smoke": 15,
	}
	for id, ceiling := range ceilings {
		job, ok := workflow.Jobs[id]
		if !ok {
			t.Fatalf("CI job %s is missing", id)
		}
		// 브랜치 보호 규칙이 check 이름으로 job을 지목하므로 이름은 고정이다.
		// matrix job은 고정된 접두사 뒤에 shard 번호만 붙는다.
		if job.Name != id && !(len(job.Strategy.Matrix) > 0 && strings.HasPrefix(job.Name, id+" (")) {
			t.Fatalf("CI job %s check name = %q, want stable %q", id, job.Name, id)
		}
		if job.TimeoutMinutes <= 0 {
			t.Fatalf("CI job %s has no timeout-minutes; an unbounded job can hang a queue", id)
		}
		if job.TimeoutMinutes > ceiling {
			t.Fatalf("CI job %s timeout = %dm exceeds ceiling %dm", id, job.TimeoutMinutes, ceiling)
		}
	}
	linter := ciStep(t, workflow.Jobs["lint"], "golangci-lint")
	if linter.Uses != "golangci/golangci-lint-action@1e7e51e771db61008b38414a730f564565cf7c20" ||
		linter.With["version"] != "v2.14.0" {
		t.Fatalf("golangci-lint gate drifted: uses=%q version=%v", linter.Uses, linter.With["version"])
	}

	// The GoReleaser fixtures download their pinned module; a transient
	// checksum-database error there must be retried before the tests run.
	// Both integration jobs run GoReleaser fixtures (pkg/ and internal/
	// companionmanifest), so each prefetches before its tests.
	for id, testStep := range map[string]string{
		"lineage-integration":  "Test release lineage integration",
		"integration-packages": "Test non-lineage integration packages",
	} {
		prefetch := ciStepRun(t, workflow.Jobs[id], "Prefetch pinned GoReleaser module")
		if !strings.Contains(prefetch, "go run "+productionGoReleaserModule+" --version") ||
			!strings.Contains(prefetch, "for attempt in 1 2 3") {
			t.Fatalf("%s: GoReleaser prefetch must retry the exact fixture module %s:\n%s", id, productionGoReleaserModule, prefetch)
		}
		if ciStepIndex(t, workflow.Jobs[id], "Prefetch pinned GoReleaser module") >= ciStepIndex(t, workflow.Jobs[id], testStep) {
			t.Fatalf("%s: GoReleaser prefetch must run before the integration tests", id)
		}
	}

	nonLineageRun := ciStepRun(t, workflow.Jobs["integration-packages"], "Test non-lineage integration packages")
	// 이 레인의 계약은 "lineage 패키지와 internal/cli를 각각 정확히 하나씩 빼고
	// 나머지를 돈다"이다 (internal/cli는 test-cli shard가 integration 태그로 돈다).
	// 스크립트 문면을 베끼는 대신 그 성질을 확인한다.
	for _, required := range []string{
		"go list -tags integration ./pkg/companionmanifest",
		"go list -tags integration ./internal/cli",
		"go list -tags integration ./...",
		"lineage_matches == 1",
		"cli_matches == 1",
		"-tags integration",
	} {
		if !strings.Contains(nonLineageRun, required) {
			t.Fatalf("non-lineage integration lane missing %q", required)
		}
	}
	// Each lineage shard builds only its own coordinates, so the matrix must
	// number its shards 1..N and hand each its index over the total; a shard
	// list with a gap would leave coordinates unbuilt and still pass.
	lineageJob := workflow.Jobs["lineage-integration"]
	shards, _ := lineageJob.Strategy.Matrix["shard"].([]any)
	for i, shard := range shards {
		if number, ok := shard.(int); !ok || number != i+1 {
			t.Fatalf("lineage-integration shards = %v, want 1..N in order", shards)
		}
	}
	if len(shards) < 1 || lineageJob.Env["AUTOPUS_LINEAGE_SHARD"] != "${{ matrix.shard }}/${{ strategy.job-total }}" {
		t.Fatalf("lineage-integration must pass AUTOPUS_LINEAGE_SHARD=<shard>/<total>, got shards=%v env=%q",
			shards, lineageJob.Env["AUTOPUS_LINEAGE_SHARD"])
	}
	lineageRun := ciStepRun(t, workflow.Jobs["lineage-integration"], "Test release lineage integration")
	for _, required := range []string{"-tags integration", "./pkg/companionmanifest"} {
		if !strings.Contains(lineageRun, required) {
			t.Fatalf("release lineage integration lane missing %q", required)
		}
	}
	// 두 레인 모두 -timeout을 들고 있어야 한다. 값은 Makefile과 달리 여기서만
	// 쓰이므로 존재만 요구한다.
	for name, run := range map[string]string{"non-lineage": nonLineageRun, "lineage": lineageRun} {
		if !strings.Contains(run, "-timeout=") {
			t.Fatalf("%s integration lane has no go test timeout", name)
		}
	}
	if strings.Contains(nonLineageRun+lineageRun, "-race") ||
		strings.Contains(nonLineageRun+lineageRun, "-coverprofile") {
		t.Fatal("CI release lineage integration gate must be isolated from race coverage instrumentation")
	}
}

func TestCIWorkflow_StaticAndMacOSContractsArePullRequestSafe(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/ci.yaml")
	for _, id := range []string{"test-isolated", "test-packages", "test-cli", "test", "lineage-integration", "integration-packages", "macos-runtime"} {
		checkout := ciUsesStep(t, workflow.Jobs[id], "actions/checkout@")
		if checkout.With["fetch-depth"] != 0 {
			t.Fatalf("CI job %s checkout fetch-depth = %v, want 0 for release ancestry checks",
				id, checkout.With["fetch-depth"])
		}
	}
	static := workflow.Jobs["static-contracts"]
	if run := ciStepRun(t, static, "Validate GitHub Actions workflows"); run != "go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7" {
		t.Fatalf("actionlint command = %q", run)
	}
	goReleaser := ciStep(t, static, "Validate GoReleaser configuration")
	if goReleaser.Uses != "goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94" ||
		goReleaser.With["version"] != "v2.17.0" || goReleaser.With["args"] != "check" {
		t.Fatalf("GoReleaser static gate drifted: uses=%q with=%v", goReleaser.Uses, goReleaser.With)
	}
	shellRun := ciStepRun(t, static, "Validate shell syntax")
	for _, required := range []string{"git ls-files -z '*.sh'", `bash -n "$script"`} {
		if !strings.Contains(shellRun, required) {
			t.Fatalf("shell static gate missing %q", required)
		}
	}
	macOS := workflow.Jobs["macos-runtime"]
	macOSRaw := readCIStabilityJob(t, ".github/workflows/ci.yaml", "macos-runtime")
	if macOS.RunsOn != "macos-15" || strings.Contains(macOSRaw, "${{ secrets.") {
		t.Fatalf("macOS PR runtime boundary runner=%q contains-secrets=%t",
			macOS.RunsOn, strings.Contains(macOSRaw, "${{ secrets."))
	}
	macRun := ciStepRun(t, macOS, "Test macOS companion runtime contracts")
	for _, required := range []string{"-timeout=8m", "./internal/companionmanifest/...", "./pkg/companionmanifest/..."} {
		if !strings.Contains(macRun, required) {
			t.Fatalf("macOS runtime gate missing %q", required)
		}
	}
	if strings.Contains(macRun, "-tags integration") {
		t.Fatal("macOS PR runtime gate must not run networked executable release fixtures")
	}
}

func TestSecurityWorkflow_JobsHaveBoundedTimeouts(t *testing.T) {
	workflow := readCIStabilityWorkflow(t, ".github/workflows/security.yml")
	// CI job과 같은 규칙이다: 예산은 유한해야 하고 상한 아래여야 한다. 값 자체를
	// 베끼지는 않는다. 반면 govulncheck 버전 핀은 아래에서 정확히 고정한다.
	ceilings := map[string]int{"gitleaks": 20, "govulncheck": 30}
	for id, ceiling := range ceilings {
		job, ok := workflow.Jobs[id]
		if !ok {
			t.Fatalf("security job %s is missing", id)
		}
		if job.TimeoutMinutes <= 0 {
			t.Fatalf("security job %s has no timeout-minutes", id)
		}
		if job.TimeoutMinutes > ceiling {
			t.Fatalf("security job %s timeout = %dm exceeds ceiling %dm", id, job.TimeoutMinutes, ceiling)
		}
	}
	install := ciStepRun(t, workflow.Jobs["govulncheck"], "Install govulncheck")
	if install != "go install golang.org/x/vuln/cmd/govulncheck@v1.6.0" {
		t.Fatalf("govulncheck install command = %q", install)
	}
	for path, mutable := range map[string]string{
		".github/workflows/ci.yaml":      "version: latest",
		".github/workflows/security.yml": "govulncheck@latest",
	} {
		if strings.Contains(readReleaseFile(t, path), mutable) {
			t.Fatalf("%s reintroduced mutable selector %q", path, mutable)
		}
	}
}

func readCIStabilityWorkflow(t *testing.T, path string) ciStabilityWorkflow {
	t.Helper()
	var workflow ciStabilityWorkflow
	if err := yaml.Unmarshal([]byte(readReleaseFile(t, path)), &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return workflow
}

func readCIStabilityJob(t *testing.T, path, id string) string {
	t.Helper()
	var workflow struct {
		Jobs map[string]any `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readReleaseFile(t, path)), &workflow); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	job, ok := workflow.Jobs[id]
	if !ok {
		t.Fatalf("CI job %s is missing", id)
	}
	encoded, err := yaml.Marshal(job)
	if err != nil {
		t.Fatalf("encode CI job %s: %v", id, err)
	}
	return string(encoded)
}

func ciStepRun(t *testing.T, job ciStabilityJob, name string) string {
	t.Helper()
	return ciStep(t, job, name).Run
}

func ciStepIndex(t *testing.T, job ciStabilityJob, name string) int {
	t.Helper()
	for index, step := range job.Steps {
		if step.Name == name {
			return index
		}
	}
	t.Fatalf("CI step %q is missing", name)
	return -1
}

func ciStep(t *testing.T, job ciStabilityJob, name string) ciStabilityStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("CI step %q is missing", name)
	return ciStabilityStep{}
}

func ciUsesStep(t *testing.T, job ciStabilityJob, prefix string) ciStabilityStep {
	t.Helper()
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, prefix) {
			return step
		}
	}
	t.Fatalf("CI uses step %q is missing", prefix)
	return ciStabilityStep{}
}
