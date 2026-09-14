package companionmanifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciStabilityStep struct {
	Name string         `yaml:"name"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

type ciStabilityJob struct {
	Name           string            `yaml:"name"`
	RunsOn         string            `yaml:"runs-on"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	Steps          []ciStabilityStep `yaml:"steps"`
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
		"test": 60, "lineage-integration": 60, "e2e": 20, "lint": 20,
		"static-contracts": 20, "macos-runtime": 30,
		"omp-native-smoke": 15,
	}
	for id, ceiling := range ceilings {
		job, ok := workflow.Jobs[id]
		if !ok {
			t.Fatalf("CI job %s is missing", id)
		}
		// 브랜치 보호 규칙이 check 이름으로 job을 지목하므로 이름은 고정이다.
		if job.Name != id {
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
		linter.With["version"] != "v2.12.2" {
		t.Fatalf("golangci-lint gate drifted: uses=%q version=%v", linter.Uses, linter.With["version"])
	}

	// 프로세스 중량 테스트 집합은 Makefile이 단일 출처다. CI가 정규식을 복사해 두면
	// 로컬 게이트와 CI 게이트가 조용히 갈라지므로, 여기서는 "복사본이 없다"를
	// 강제한다: 선택자는 Makefile에서 읽혀야 하고, 파싱 실패는 즉시 에러여야 한다.
	resolver := ciStepRun(t, workflow.Jobs["test"], "Resolve process-heavy test selector")
	for _, required := range []string{
		"set -euo pipefail",
		"sed -n 's/^PROCESS_HEAVY_TESTS ?= //p' Makefile",
		`[[ -n "$selector" ]]`,
		"$GITHUB_ENV",
	} {
		if !strings.Contains(resolver, required) {
			t.Fatalf("process-heavy selector must derive from the Makefile; missing %q", required)
		}
	}

	isolated := ciStepRun(t, workflow.Jobs["test"], "Test process-heavy contracts in isolation")
	shared := ciStepRun(t, workflow.Jobs["test"], "Test with Coverage")
	// 두 레인은 같은 선택자를 반대 방향으로 쓴다. 한쪽이 리터럴을 들고 있으면
	// 집합이 갈라지므로, 양쪽 모두 환경 변수를 경유해야 한다.
	if !strings.Contains(isolated, `-run "$PROCESS_HEAVY_TESTS"`) {
		t.Fatal("isolated lane must run exactly the resolved process-heavy selector")
	}
	if !strings.Contains(shared, `-skip "$PROCESS_HEAVY_TESTS"`) {
		t.Fatal("shared lane must skip exactly the resolved process-heavy selector")
	}
	if strings.Contains(isolated, "-skip") || strings.Contains(shared, "-run ") {
		t.Fatal("process-heavy run/skip boundary is not exact")
	}
	// 격리의 목적은 공유 스케줄러에서 떼어내는 것이므로 -p 1이 본질이고,
	// race 게이트는 격리 때문에 약해지면 안 된다.
	if !strings.Contains(isolated, "-p 1") || !strings.Contains(isolated, "-race") {
		t.Fatal("isolated lane must serialise packages while keeping the race gate")
	}
	// 격리 레인이 조용히 비어도 통과하는 일을 막는 기존 인벤토리 가드는 유지한다.
	if !strings.Contains(isolated, "TestReleaseHardeningBashContract") {
		t.Fatal("isolated lane lost its inventory guard against an empty selector")
	}
	if ciStepIndex(t, workflow.Jobs["test"], "Test process-heavy contracts in isolation") >=
		ciStepIndex(t, workflow.Jobs["test"], "Test with Coverage") {
		t.Fatal("process-heavy contracts must pass before shared race coverage")
	}
	// 건너뛴 테스트의 커버리지는 격리 레인 프로파일에 있으므로 합쳐야 총계가 산다.
	for _, required := range []string{"coverage-isolated.out", "coverage-shared.out", "coverage.out"} {
		if !strings.Contains(isolated+shared, required) {
			t.Fatalf("split lanes must merge coverage profiles; missing %q", required)
		}
	}
	if !strings.Contains(readReleaseFile(t, ".github/workflows/ci.yaml"), "COVERAGE_THRESHOLD: \"85\"") {
		t.Fatal("coverage threshold is not pinned")
	}
	if !strings.Contains(shared, "./...") {
		t.Fatal("CI race coverage gate must cover every Go package")
	}
	if strings.Contains(shared, "-tags integration") {
		t.Fatal("CI race coverage gate must not run executable release integration fixtures")
	}

	nonLineageRun := ciStepRun(t, workflow.Jobs["lineage-integration"], "Test non-lineage integration packages")
	// 이 레인의 계약은 "lineage 패키지를 정확히 하나 빼고 나머지를 돈다"이다.
	// 스크립트 문면을 베끼는 대신 그 성질을 확인한다.
	for _, required := range []string{
		"go list -tags integration ./pkg/companionmanifest",
		"go list -tags integration ./...",
		"lineage_matches == 1",
		"-tags integration",
	} {
		if !strings.Contains(nonLineageRun, required) {
			t.Fatalf("non-lineage integration lane missing %q", required)
		}
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
	for _, id := range []string{"test", "lineage-integration", "macos-runtime"} {
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
