package gates

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ChangeContractSchema is the schema id carried by a compact contract.
	ChangeContractSchema = "autopus.change-contract.v1"
	// ChangeContractFile is the document name a compact contract uses.
	ChangeContractFile = "change.md"
	// changeContractDirPrefix marks a sibling directory holding a contract
	// that references a SPEC stored elsewhere.
	changeContractDirPrefix = "CHG-"
	// maxChangeContractBytes bounds the document this package will read. A
	// contract is a page of references; anything larger is not one.
	maxChangeContractBytes = 64 << 10
	// harnessStateRoot holds the SPEC set, contracts, receipts, and
	// checkpoints. It is the contract's own bookkeeping, not the
	// implementation surface a contract bounds.
	harnessStateRoot = ".autopus/"
)

// ChangeContract is a compact change contract recorded against an existing
// SPEC. Recorded carries the risk decision the document states; Risk carries
// the decision recomputed from the document's own declaration and surface.
// The recomputed one is the authority: a recorded tier is a claim.
type ChangeContract struct {
	Path             string
	SpecID           string
	ChangeID         string
	AcceptanceIDs    []string
	Surface          []string
	VerificationPlan []string
	NewContract      bool
	Recorded         ChangeRisk
	Risk             ChangeRisk
}

// CompactRouteDecision is the authorization outcome for the compact route.
// Reason is always populated, so a run can say why it took the route it did.
type CompactRouteDecision struct {
	Authorized bool
	Reason     string
	Contract   ChangeContract
}

// AuthorizeCompactRoute decides whether the compact route is authorized for
// specID. Every condition has to hold: exactly one contract references the
// SPEC, the contract parses, the risk recomputed from its declaration and
// declared surface is low, the risk it recorded agrees with that
// recomputation, and the paths actually changed both stay inside the declared
// surface and reassess as low risk under the same declaration. Anything else
// keeps the full route, because a shortened route is a claim about risk that
// only complete evidence can support.
func AuthorizeCompactRoute(specDir, specID string, uiGlobs, changed []string) CompactRouteDecision {
	contract, found, err := FindChangeContract(specDir, specID, uiGlobs)
	switch {
	case err != nil:
		return CompactRouteDecision{Reason: "change contract unusable: " + err.Error()}
	case !found:
		return CompactRouteDecision{Reason: "no compact change contract for this SPEC"}
	}
	decision := CompactRouteDecision{Contract: contract}
	if !contract.Recorded.Compact() {
		decision.Reason = fmt.Sprintf("%s records %s risk %s (declared %s, effective %s)",
			contract.DisplayPath(), contract.Recorded.Tier, contract.Recorded.Decision,
			contract.Recorded.DeclaredClass, contract.Recorded.EffectiveClass)
		return decision
	}
	if !contract.Risk.Compact() {
		decision.Reason = fmt.Sprintf(
			"%s declares %s but its intended surface reassesses as %s %s: %s",
			contract.DisplayPath(), contract.Recorded.DeclaredClass, contract.Risk.Tier,
			contract.Risk.EffectiveClass, strings.Join(contract.Risk.Reasons, ", "))
		return decision
	}
	if contract.Risk.EffectiveClass != contract.Recorded.EffectiveClass {
		decision.Reason = fmt.Sprintf(
			"%s records effective class %s but its surface reassesses as %s",
			contract.DisplayPath(), contract.Recorded.EffectiveClass, contract.Risk.EffectiveClass)
		return decision
	}
	workload := ProductionChangeSet(changed)
	if outside := contract.PathsOutsideSurface(workload); len(outside) > 0 {
		decision.Reason = fmt.Sprintf(
			"%d changed path(s) fall outside the surface %s declares, starting with %s",
			len(outside), contract.DisplayPath(), outside[0])
		return decision
	}
	// A coarse surface entry can cover paths the declared class cannot. The
	// tree is assessed under the same declaration, so a change that spans a
	// second module root or reaches a security surface keeps the full route
	// even while staying formally inside the declared surface.
	actual := AssessChange(contract.Recorded.DeclaredClass, Classify(workload, uiGlobs), contract.NewContract)
	if !actual.Compact() {
		decision.Reason = fmt.Sprintf(
			"the %d path(s) actually changed reassess as %s %s under the declared %s: %s",
			len(workload), actual.Tier, actual.EffectiveClass, contract.Recorded.DeclaredClass,
			strings.Join(actual.Reasons, ", "))
		return decision
	}
	decision.Authorized = true
	decision.Reason = fmt.Sprintf(
		"%s authorizes a low-risk %s change over %d declared path(s)",
		contract.DisplayPath(), contract.Risk.EffectiveClass, len(contract.Surface))
	return decision
}

// DisplayPath is the contract path in slash form, for reasons an operator reads.
func (c ChangeContract) DisplayPath() string { return filepath.ToSlash(c.Path) }

// PathsOutsideSurface returns the changed paths the declared surface does not
// cover. A surface entry covers itself and everything beneath it.
func (c ChangeContract) PathsOutsideSurface(paths []string) []string {
	var outside []string
	for _, path := range ProductionChangeSet(paths) {
		if !c.coversPath(path) {
			outside = append(outside, path)
		}
	}
	return outside
}

// ProductionChangeSet normalizes a change set and drops harness bookkeeping
// under .autopus/: the contract, its gate receipts, and the run's own
// checkpoints are records of a change, not the change itself.
func ProductionChangeSet(paths []string) []string {
	workload := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := filepath.ToSlash(strings.TrimSpace(raw))
		if path == "" || strings.HasPrefix(path, harnessStateRoot) {
			continue
		}
		workload = append(workload, path)
	}
	return workload
}

func (c ChangeContract) coversPath(path string) bool {
	for _, entry := range c.Surface {
		if path == entry || strings.HasPrefix(path, entry+"/") {
			return true
		}
	}
	return false
}

// FindChangeContract returns the single compact contract recorded for specID:
// {specDir}/change.md, or a sibling CHG-<id>/change.md that names the SPEC. A
// missing contract is reported as found=false; two contracts naming the same
// SPEC are ambiguous and refused, because picking one of them would let an
// unrelated document decide how a run executes.
func FindChangeContract(specDir, specID string, uiGlobs []string) (ChangeContract, bool, error) {
	var matches []ChangeContract
	own, found, err := readChangeContract(filepath.Join(specDir, ChangeContractFile), uiGlobs)
	if err != nil {
		return ChangeContract{}, false, err
	}
	if found {
		if own.SpecID != specID {
			return ChangeContract{}, false, fmt.Errorf(
				"change contract %s references SPEC %s, not %s", own.DisplayPath(), own.SpecID, specID)
		}
		matches = append(matches, own)
	}
	siblings, err := siblingChangeContracts(filepath.Dir(specDir), specID, uiGlobs)
	if err != nil {
		return ChangeContract{}, false, err
	}
	matches = append(matches, siblings...)
	switch len(matches) {
	case 0:
		return ChangeContract{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return ChangeContract{}, false, fmt.Errorf(
			"SPEC %s is referenced by %d change contracts (%s): resolve the duplicate before running",
			specID, len(matches), strings.Join(changeContractPaths(matches), ", "))
	}
}

// siblingChangeContracts collects every CHG-<id>/change.md under specsRoot
// that names specID. A candidate that cannot be read is an error, not a
// silent skip: a damaged contract beside a SPEC is exactly the case where
// guessing is unsafe.
func siblingChangeContracts(specsRoot, specID string, uiGlobs []string) ([]ChangeContract, error) {
	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read SPEC root %s: %w", filepath.ToSlash(specsRoot), err)
	}
	var matches []ChangeContract
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), changeContractDirPrefix) {
			continue
		}
		path := filepath.Join(specsRoot, entry.Name(), ChangeContractFile)
		contract, found, readErr := readChangeContract(path, uiGlobs)
		if readErr != nil {
			return nil, readErr
		}
		if found && contract.SpecID == specID {
			matches = append(matches, contract)
		}
	}
	return matches, nil
}

func changeContractPaths(contracts []ChangeContract) []string {
	paths := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		paths = append(paths, contract.DisplayPath())
	}
	return paths
}

// readChangeContract loads and reassesses one contract document. An absent
// file is not an error; a symlink, an oversized body, a malformed header, or
// a body missing the material that bounds a run is.
func readChangeContract(path string, uiGlobs []string) (ChangeContract, bool, error) {
	document, found, err := readChangeContractDocument(path)
	if err != nil || !found {
		return ChangeContract{}, false, err
	}
	header, body, err := parseChangeContract(document)
	if err != nil {
		return ChangeContract{}, false, fmt.Errorf("change contract %s: %w", filepath.ToSlash(path), err)
	}
	if body.SpecBinding != header.SpecID {
		return ChangeContract{}, false, fmt.Errorf(
			"change contract %s: header names SPEC %s but the body references %s",
			filepath.ToSlash(path), header.SpecID, body.SpecBinding)
	}
	classification := Classify(body.Surface, uiGlobs)
	return ChangeContract{
		Path:             path,
		SpecID:           header.SpecID,
		ChangeID:         header.ChangeID,
		AcceptanceIDs:    body.AcceptanceIDs,
		Surface:          body.Surface,
		VerificationPlan: body.VerificationPlan,
		NewContract:      header.NewContract,
		Recorded: ChangeRisk{
			DeclaredClass:  header.DeclaredClass,
			EffectiveClass: header.EffectiveClass,
			Tier:           header.Tier,
			Decision:       header.Decision,
		},
		Risk: AssessChange(header.DeclaredClass, classification, header.NewContract),
	}, true, nil
}

// readChangeContractDocument reads a bounded, regular contract file.
func readChangeContractDocument(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stat change contract %s: %w", filepath.ToSlash(path), err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return "", false, fmt.Errorf("change contract %s is a symlink", filepath.ToSlash(path))
	case !info.Mode().IsRegular():
		return "", false, fmt.Errorf("change contract %s is not a regular file", filepath.ToSlash(path))
	case info.Size() > maxChangeContractBytes:
		return "", false, fmt.Errorf(
			"change contract %s is %d bytes, over the %d byte limit",
			filepath.ToSlash(path), info.Size(), maxChangeContractBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, fmt.Errorf("open change contract %s: %w", filepath.ToSlash(path), err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", false, fmt.Errorf("change contract identity changed: %s", filepath.ToSlash(path))
	}
	body, err := io.ReadAll(io.LimitReader(file, maxChangeContractBytes+1))
	if err != nil {
		return "", false, fmt.Errorf("read change contract %s: %w", filepath.ToSlash(path), err)
	}
	if len(body) > maxChangeContractBytes {
		return "", false, fmt.Errorf("change contract %s exceeds the byte limit", filepath.ToSlash(path))
	}
	return string(body), true, nil
}
