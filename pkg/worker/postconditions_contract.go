package worker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// completionContractMessage is the optional explicit completion contract of a
// task payload (SPEC-WORKERPOST-001 REQ-WPOST-INPUT-01). Its presence switches
// prompt inference off: the worker checks exactly what the contract states.
// The contract grants nothing; the worker never performs the actions it names.
type completionContractMessage struct {
	Version          int      `json:"version,omitempty"`
	RequiredActions  []string `json:"required_actions,omitempty"`
	ForbiddenActions []string `json:"forbidden_actions,omitempty"`
	Remote           string   `json:"remote,omitempty"`
	Ref              string   `json:"ref,omitempty"`
	ExpectedCommit   string   `json:"expected_commit,omitempty"`
	PushMatch        string   `json:"push_match,omitempty"`
}

// requirementLevel is how a task treats one git action.
type requirementLevel string

const (
	levelNone      requirementLevel = "none"
	levelRequired  requirementLevel = "required"
	levelForbidden requirementLevel = "forbidden"
	// levelAmbiguous exists only in fallback mode: the prompt both asks for
	// and declines the action (REQ-WPOST-LEGACY-01).
	levelAmbiguous requirementLevel = "ambiguous"
)

const (
	actionCommit = "commit"
	actionPush   = "push"
	actionBranch = "branch"

	sourceContract = "contract"
	sourceLegacy   = "legacy_prompt"

	matchExact    = "exact"
	matchContains = "contains"

	defaultPostconditionRemote = "origin"
	branchRefPrefix            = "refs/heads/"

	// contractValueClip bounds every sender value quoted in an error.
	contractValueClip = 64
)

var (
	remoteNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	branchNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	commitSHAPattern  = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// taskPostconditions is the normalized plan the worker verifies after a task
// runs. Contract mode and fallback mode both produce it, so the checks never
// read the prompt themselves.
type taskPostconditions struct {
	Source         string
	Commit         requirementLevel
	Push           requirementLevel
	Branch         requirementLevel
	Remote         string   // remote name; never a URL
	Ref            string   // full refs/heads/ ref; contract mode only
	ExpectedCommit string   // full lowercase SHA; contract mode only
	Match          string   // matchExact or matchContains
	Branches       []string // fallback mode: branch names the prompt names
	Mentioned      bool     // fallback mode: the prompt mentions an action
	Problems       []string // contract mode: why the contract is invalid
}

// planTaskPostconditions builds the plan for one task. A contract is
// authoritative (REQ-WPOST-INPUT-03); without one the prompt fallback applies.
func planTaskPostconditions(contract *completionContractMessage, prompt string) taskPostconditions {
	if contract == nil {
		return detectTaskPostconditions(prompt)
	}
	return planFromContract(*contract)
}

// planPayloadPostconditions plans a received task and rejects an invalid
// contract before any provider runs (REQ-WPOST-INPUT-02): a result that can
// never be judged is not worth a model call or a possible publication.
func planPayloadPostconditions(contract *completionContractMessage, prompt string) (*taskPostconditions, error) {
	plan := planTaskPostconditions(contract, prompt)
	if len(plan.Problems) > 0 {
		return nil, fmt.Errorf("invalid completion_contract: %s", strings.Join(plan.Problems, "; "))
	}
	return &plan, nil
}

// postconditionPlan returns the plan handleTask attached to the task, or the
// prompt fallback for callers that run a task without one.
func (m taskRunMeta) postconditionPlan(prompt string) taskPostconditions {
	if m.Postconditions != nil {
		return *m.Postconditions
	}
	return detectTaskPostconditions(prompt)
}

func planFromContract(c completionContractMessage) taskPostconditions {
	plan := taskPostconditions{
		Source: sourceContract,
		Remote: defaultPostconditionRemote,
		Match:  matchExact,
	}
	if c.Version != 0 && c.Version != 1 {
		plan.Problems = append(plan.Problems, fmt.Sprintf("unsupported version %d", c.Version))
	}

	levels := make(map[string]requirementLevel)
	plan.Problems = append(plan.Problems, collectContractActions(levels, c.RequiredActions, levelRequired)...)
	plan.Problems = append(plan.Problems, collectContractActions(levels, c.ForbiddenActions, levelForbidden)...)
	plan.Commit = contractLevel(levels, actionCommit)
	plan.Push = contractLevel(levels, actionPush)
	plan.Branch = contractLevel(levels, actionBranch)

	if remote := strings.TrimSpace(c.Remote); remote != "" {
		if remoteNamePattern.MatchString(remote) {
			plan.Remote = remote
		} else {
			plan.Problems = append(plan.Problems, "invalid remote "+quoteClip(c.Remote))
		}
	}
	ref, problem := normalizeContractRef(c.Ref)
	if problem != "" {
		plan.Problems = append(plan.Problems, problem)
	}
	plan.Ref = ref
	if expected := strings.ToLower(strings.TrimSpace(c.ExpectedCommit)); expected != "" {
		if commitSHAPattern.MatchString(expected) {
			plan.ExpectedCommit = expected
		} else {
			plan.Problems = append(plan.Problems,
				"expected_commit must be a full 40 or 64 character hex SHA, got "+quoteClip(c.ExpectedCommit))
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.PushMatch)) {
	case "", matchExact:
	case matchContains:
		plan.Match = matchContains
	default:
		plan.Problems = append(plan.Problems, "unsupported push_match "+quoteClip(c.PushMatch))
	}
	return plan
}

// collectContractActions records one list's actions at level and reports
// unknown names and actions that appear in both lists.
func collectContractActions(levels map[string]requirementLevel, names []string, level requirementLevel) []string {
	var problems []string
	for _, raw := range names {
		name := strings.ToLower(strings.TrimSpace(raw))
		switch name {
		case actionCommit, actionPush, actionBranch:
		default:
			problems = append(problems, "unsupported action "+quoteClip(raw))
			continue
		}
		if previous, seen := levels[name]; seen && previous != level {
			problems = append(problems, fmt.Sprintf("action %q is both required and forbidden", name))
		}
		levels[name] = level
	}
	return problems
}

func contractLevel(levels map[string]requirementLevel, action string) requirementLevel {
	if level, ok := levels[action]; ok {
		return level
	}
	return levelNone
}

// normalizeContractRef turns a bare branch name or a refs/heads/ ref into the
// full ref, or names why it cannot be checked.
func normalizeContractRef(raw string) (string, string) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", ""
	}
	name := ref
	if strings.HasPrefix(ref, "refs/") {
		if !strings.HasPrefix(ref, branchRefPrefix) {
			return "", fmt.Sprintf("ref %s is not under refs/heads/", quoteClip(raw))
		}
		name = strings.TrimPrefix(ref, branchRefPrefix)
	}
	if !validBranchName(name) {
		return "", "invalid branch name " + quoteClip(raw)
	}
	return branchRefPrefix + name, ""
}

// validBranchName accepts a conservative subset of git branch names: nothing
// git could read as an option, a range, or a lock file.
func validBranchName(name string) bool {
	if !branchNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return false
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasPrefix(part, "-") {
			return false
		}
	}
	return true
}

// quoteClip quotes a sender value for an error message, clipped on a rune
// boundary, so a payload can neither flood nor forge a log line.
func quoteClip(value string) string {
	if len(value) > contractValueClip {
		cut := contractValueClip
		for cut > 0 && !utf8.RuneStart(value[cut]) {
			cut--
		}
		value = value[:cut]
	}
	return strconv.Quote(value)
}
