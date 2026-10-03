package compile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"gopkg.in/yaml.v3"
)

type Candidate struct {
	JourneyID         string         `json:"journey_id"`
	StepID            string         `json:"step_id"`
	Adapter           string         `json:"adapter"`
	Command           []string       `json:"command,omitempty"`
	CWD               string         `json:"cwd,omitempty"`
	Timeout           string         `json:"timeout,omitempty"`
	EnvAllowlist      []string       `json:"env_allowlist,omitempty"`
	Artifacts         []string       `json:"artifacts,omitempty"`
	AcceptanceRefs    []string       `json:"acceptance_refs,omitempty"`
	Source            string         `json:"source"`
	InputSource       string         `json:"input_source,omitempty"`
	PassFailAuthority string         `json:"pass_fail_authority,omitempty"`
	OracleThresholds  map[string]any `json:"oracle_thresholds,omitempty"`
	ManualOrDeferred  bool           `json:"manual_or_deferred,omitempty"`
	ErrorCode         string         `json:"error_code,omitempty"`
}

type qameshCheck struct {
	Adapter           string         `yaml:"adapter"`
	Command           []string       `yaml:"command"`
	CWD               string         `yaml:"cwd"`
	Timeout           string         `yaml:"timeout"`
	Env               []string       `yaml:"env"`
	EnvAllowlist      []string       `yaml:"env_allowlist"`
	Artifacts         []string       `yaml:"artifacts"`
	AcceptanceRefs    []string       `yaml:"acceptance_refs"`
	Source            string         `yaml:"source"`
	PassFailAuthority string         `yaml:"pass_fail_authority"`
	Expected          map[string]any `yaml:"expected"`
}

func FromProject(projectDir string) []Candidate {
	var candidates []Candidate
	candidates = append(candidates, fromScenarios(filepath.Join(projectDir, ".autopus", "project", "scenarios.md"), projectDir)...)
	specsDir := filepath.Join(projectDir, ".autopus", "specs")
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != "acceptance.md" {
			return nil
		}
		candidates = append(candidates, fromAcceptance(path, projectDir)...)
		return nil
	})
	candidates = append(candidates, fromTestScenarios(projectDir)...)
	return candidates
}

func fromScenarios(path, projectDir string) []Candidate {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var out []Candidate
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "command:") && !strings.HasPrefix(lower, "- command:") {
			continue
		}
		_, command, _ := strings.Cut(line, ":")
		command = strings.Trim(strings.TrimSpace(command), "` ")
		candidate := candidateFromCommand("scenario", "", strings.Fields(command), nil, projectDir)
		out = append(out, candidate)
	}
	return out
}

func fromAcceptance(path, projectDir string) []Candidate {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(body), "\n")
	var out []Candidate
	for index := 0; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) != "```qamesh-check" {
			continue
		}
		var block []string
		for index++; index < len(lines) && strings.TrimSpace(lines[index]) != "```"; index++ {
			block = append(block, lines[index])
		}
		var check qameshCheck
		if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &check); err != nil {
			out = append(out, Candidate{Source: "compiled", ManualOrDeferred: true, ErrorCode: "qa_compiler_parse_invalid"})
			continue
		}
		out = append(out, candidateFromCommand("acceptance", check.Adapter, check.Command, check, projectDir))
	}
	return out
}

func candidateFromCommand(source, adapterID string, argv []string, check any, projectDir string) Candidate {
	if adapterID == "" {
		adapterID = inferAdapter(argv)
	}
	cmd := journey.Command{Argv: argv, CWD: ".", Timeout: "60s"}
	var refs []string
	var thresholds map[string]any
	var artifacts []string
	var inputSource string
	var passFailAuthority string
	if typed, ok := check.(qameshCheck); ok {
		cmd.CWD = typed.CWD
		cmd.Timeout = typed.Timeout
		cmd.EnvAllowlist = typed.EnvAllowlist
		refs = typed.AcceptanceRefs
		thresholds = typed.Expected
		artifacts = typed.Artifacts
		inputSource = typed.Source
		passFailAuthority = typed.PassFailAuthority
		if shouldDeferToQAMESH003(adapterID, inputSource, passFailAuthority) {
			return deferredCandidate(source, adapterID, argv, refs, inputSource, passFailAuthority)
		}
		if len(typed.Env) > 0 {
			return Candidate{Source: "compiled", Adapter: adapterID, ManualOrDeferred: true, ErrorCode: "qa_compiler_env_not_allowlisted"}
		}
	}
	if shouldDeferToQAMESH003(adapterID, inputSource, passFailAuthority) {
		return deferredCandidate(source, adapterID, argv, refs, inputSource, passFailAuthority)
	}
	if cmd.CWD == "" {
		cmd.CWD = "."
	}
	if cmd.Timeout == "" {
		cmd.Timeout = "60s"
	}
	if err := journey.ValidateCompiledCommand(adapterID, cmd, artifactRefs(artifacts), projectDir); err != nil {
		code := "qa_compiler_command_unsafe"
		if validationErr, ok := err.(*journey.ValidationError); ok {
			code = validationErr.Code
		}
		return Candidate{Source: "compiled", ManualOrDeferred: true, ErrorCode: code}
	}
	return Candidate{
		JourneyID:         journeyID(source, adapterID),
		StepID:            "step-1",
		Adapter:           adapterID,
		Command:           argv,
		CWD:               cmd.CWD,
		Timeout:           cmd.Timeout,
		EnvAllowlist:      cmd.EnvAllowlist,
		Artifacts:         artifacts,
		AcceptanceRefs:    refs,
		Source:            "compiled",
		InputSource:       inputSource,
		PassFailAuthority: passFailAuthority,
		OracleThresholds:  thresholds,
	}
}

func deferredCandidate(source, adapterID string, argv []string, refs []string, inputSource, passFailAuthority string) Candidate {
	return Candidate{
		JourneyID:         journeyID(source, adapterID),
		StepID:            "step-1",
		Adapter:           adapterID,
		Command:           argv,
		AcceptanceRefs:    refs,
		Source:            "compiled",
		InputSource:       inputSource,
		PassFailAuthority: passFailAuthority,
		ManualOrDeferred:  true,
		ErrorCode:         "qa_compiler_deferred_to_SPEC-QAMESH-003",
	}
}

// journeyID is the one place a compiled candidate's journey id is derived;
// run plans, filters, and output directories are all keyed by it.
func journeyID(source, adapterID string) string {
	return "compiled-" + source + "-" + adapterID
}

func artifactRefs(values []string) []journey.Artifact {
	out := make([]journey.Artifact, 0, len(values))
	for _, value := range values {
		out = append(out, journey.Artifact{Path: value})
	}
	return out
}

func shouldDeferToQAMESH003(adapterID, inputSource, passFailAuthority string) bool {
	if strings.EqualFold(strings.TrimSpace(passFailAuthority), "ai") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(inputSource), "production_session") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(adapterID)) {
	case "browserstack", "firebase-test-lab", "maestro", "detox", "session-replay":
		return true
	default:
		return false
	}
}

func inferAdapter(argv []string) string {
	if len(argv) >= 2 && argv[0] == "go" && argv[1] == "test" {
		return "go-test"
	}
	if len(argv) > 0 && argv[0] == "pytest" {
		return "pytest"
	}
	return "custom-command"
}
