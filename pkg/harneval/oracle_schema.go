package harneval

import (
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Oracle modes of an agent task (SPEC-HARNEVAL-003 REQ-HR-08, the CD-HR-5
// revision of the SPEC-HARNEVAL-001 task schema). An absent mode is
// white_box: the corpus oracle grades the task in the advisory lane only. A
// black_box task also carries the black_box_oracle the signed lane judges it
// with, and only a black_box task may carry one.
const (
	OracleModeWhiteBox = "white_box"
	OracleModeBlackBox = "black_box"
)

// OracleFixtureRoot holds every black-box input fixture and expected output.
// It lies below evals/harness, which the trusted runner removes from every
// agent, build, and artifact snapshot, so no trial can read an expectation.
const OracleFixtureRoot = EvalRoot + "/oracles"

// Black-box assertion kinds (closed set).
const (
	BlackBoxExitCode = "exit_code"
	BlackBoxStdout   = "stdout"
	BlackBoxFile     = "file"
)

// Limits the trusted runner (golden_blackbox.py) applies to a definition and
// its fixtures: assertions per task, bytes per input, bytes per expected output.
const (
	maxBlackBoxAssertions = 32
	maxOracleInputBytes   = 16 << 20
	maxOracleOutputBytes  = 1 << 20
)

// BlackBoxOracle is the black_box_oracle of an agent task: the package the
// artifact is built from, its argv ({artifact} first; {input} and {output}
// name the trial input directory and the output root), the inputs copied
// into the input directory under their file names, the input fed on stdin,
// and the pinned assertions.
type BlackBoxOracle struct {
	Build      string              `json:"build"`
	Command    []string            `json:"command"`
	Inputs     []PinnedFile        `json:"inputs"`
	Stdin      *string             `json:"stdin,omitempty"`
	Assertions []BlackBoxAssertion `json:"assertions"`
}

// PinnedFile is one committed fixture pinned by the SHA-256 of its raw bytes.
type PinnedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// BlackBoxAssertion is one pinned expectation: the artifact's exit status, its
// stdout, or one output file at a fixed path below the output root. Fields a
// kind does not use stay absent.
type BlackBoxAssertion struct {
	ID       string      `json:"id"`
	Kind     string      `json:"kind"`
	ExitCode *int        `json:"exit_code,omitempty"`
	Path     *string     `json:"path,omitempty"`
	Expected *PinnedFile `json:"expected,omitempty"`
}

var (
	assertionIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	buildPackagePattern = regexp.MustCompile(`^\./[A-Za-z0-9_][A-Za-z0-9_./-]*$`)
	outputPathPattern   = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`)
)

// validateOracleFields applies the black-box rules of the trusted runner to
// one task: a known mode, nothing on a surface task, and a black_box_oracle
// exactly on a black_box task.
func validateOracleFields(t Task) error {
	mode, oracle := t.OracleMode, t.BlackBoxOracle
	switch {
	case mode != "" && mode != OracleModeWhiteBox && mode != OracleModeBlackBox:
		return invalidf(DetailFieldInvalid, "task %q oracle_mode %q is neither %s nor %s", t.ID, mode, OracleModeWhiteBox, OracleModeBlackBox)
	case t.Kind == KindSurface && (mode != "" || oracle != nil):
		return invalidf(DetailFieldInvalid, "surface task %q cannot carry oracle_mode or black_box_oracle", t.ID)
	case (mode == OracleModeBlackBox) != (oracle != nil):
		return invalidf(DetailFieldInvalid, "task %q: oracle_mode black_box needs a black_box_oracle, and no other mode may carry one", t.ID)
	case oracle == nil:
		return nil
	}
	if err := oracle.validate(); err != nil {
		return fmt.Errorf("task %q black_box_oracle: %w", t.ID, err)
	}
	return nil
}

func (o *BlackBoxOracle) validate() error {
	if !buildPackagePattern.MatchString(o.Build) || slices.Contains(strings.Split(o.Build, "/"), "..") {
		return invalidf(DetailFieldInvalid, "build %q is not a ./ package path", o.Build)
	}
	if len(o.Command) == 0 || o.Command[0] != "{artifact}" ||
		slices.ContainsFunc(o.Command[1:], func(arg string) bool { return strings.Contains(arg, "{artifact}") }) {
		return invalidf(DetailFieldInvalid, "command must start with {artifact} and name it once")
	}
	if o.Inputs == nil {
		return invalidf(DetailFieldInvalid, "inputs must be a list of {path, sha256}")
	}
	names := map[string]bool{}
	for _, input := range o.Inputs {
		if err := input.validate("input"); err != nil {
			return err
		}
		if name := path.Base(input.Path); !names[name] {
			names[name] = true
			continue
		}
		return invalidf(DetailFieldInvalid, "input file name %q repeats", path.Base(input.Path))
	}
	if o.Stdin != nil && !names[*o.Stdin] {
		return invalidf(DetailFieldInvalid, "stdin %q names no input", *o.Stdin)
	}
	if len(o.Assertions) == 0 || len(o.Assertions) > maxBlackBoxAssertions {
		return invalidf(DetailFieldInvalid, "needs 1 to %d assertions", maxBlackBoxAssertions)
	}
	ids := map[string]bool{}
	for _, assertion := range o.Assertions {
		if err := assertion.validate(); err != nil {
			return err
		}
		if ids[assertion.ID] {
			return invalidf(DetailFieldInvalid, "assertion id %q repeats", assertion.ID)
		}
		ids[assertion.ID] = true
	}
	return nil
}

func (a BlackBoxAssertion) validate() error {
	if !assertionIDPattern.MatchString(a.ID) {
		return invalidf(DetailFieldInvalid, "assertion id %q is malformed", a.ID)
	}
	var fits bool
	switch a.Kind {
	case BlackBoxExitCode:
		fits = a.ExitCode != nil && a.Path == nil && a.Expected == nil
	case BlackBoxStdout:
		fits = a.ExitCode == nil && a.Path == nil && a.Expected != nil
	case BlackBoxFile:
		fits = a.ExitCode == nil && a.Path != nil && a.Expected != nil && outputPathPattern.MatchString(*a.Path) &&
			!slices.ContainsFunc(strings.Split(*a.Path, "/"), func(part string) bool { return part == "." || part == ".." })
	}
	if !fits {
		return invalidf(DetailFieldInvalid, "assertion %q must be one of %s, %s, %s with its own fields",
			a.ID, BlackBoxExitCode, BlackBoxStdout, BlackBoxFile)
	}
	if a.Expected != nil {
		return a.Expected.validate("assertion " + a.ID + " expected output")
	}
	return nil
}

func (p PinnedFile) validate(role string) error {
	if !isCleanRelPath(p.Path) || !strings.HasPrefix(p.Path, OracleFixtureRoot+"/") {
		return invalidf(DetailUncleanPath, "%s %q is not a clean path under %s/", role, p.Path, OracleFixtureRoot)
	}
	if !sha256Pattern.MatchString(p.SHA256) {
		return invalidf(DetailFieldInvalid, "%s %s needs a lowercase 64-hex sha256", role, p.Path)
	}
	return nil
}

// checkOracleFixtures reads every fixture a black-box definition pins and
// compares its raw-byte SHA-256 and size with the pin, as the trusted runner
// does before a trial: an input up to 16 MiB, an expected output up to 1 MiB.
func (l *taskLoader) checkOracleFixtures(oracle *BlackBoxOracle) error {
	if oracle == nil {
		return nil
	}
	pins := map[PinnedFile]int{}
	for _, input := range oracle.Inputs {
		pins[input] = maxOracleInputBytes
	}
	for _, assertion := range oracle.Assertions {
		if assertion.Expected != nil {
			pins[*assertion.Expected] = maxOracleOutputBytes
		}
	}
	for _, pin := range slices.SortedFunc(maps.Keys(pins), func(a, b PinnedFile) int { return strings.Compare(a.Path, b.Path) }) {
		limit := pins[pin]
		data, err := readRepoFile(l.root, pin.Path)
		if err != nil {
			return withPath(err, pin.Path)
		}
		if sum := sha256Hex(data); sum != pin.SHA256 || len(data) > limit {
			return invalidf(DetailOracleDigestMismatch, "oracle fixture %s has sha256 %s and %d bytes, the task pins %s within %d bytes",
				pin.Path, sum, len(data), pin.SHA256, limit)
		}
	}
	return nil
}

// OracleAssertionIDs is the trusted black-box assertion ids of every active
// black-box agent task in the set, in definition order: the
// TrustedInputs.OracleAssertions the signer takes from main's task definitions.
func OracleAssertionIDs(set *Set) map[string][]string {
	ids := map[string][]string{}
	for _, task := range set.Tasks {
		if task.Kind != KindAgent || task.Status.State != StateActive || task.BlackBoxOracle == nil {
			continue
		}
		for _, assertion := range task.BlackBoxOracle.Assertions {
			ids[task.ID] = append(ids[task.ID], assertion.ID)
		}
	}
	return ids
}
