package spec

import (
	"maps"
	"slices"
	"strings"
)

// headerUnit stands for the lines of a document before its first unit.
const headerUnit = -1

// traceabilitySection is the spec.md section whose rows link the ids of a
// scope closure; a request carries its matching rows, not the whole table.
const traceabilitySection = "traceability matrix"

// deltaScopeIDs are the ids a stateless reviewer must be able to read in
// the request: the scope closure of every unresolved finding and every id a
// changed line names, removed or added (REQ-RR-DELTA-04).
func (p DeltaPlan) deltaScopeIDs() map[string]bool {
	ids := map[string]bool{}
	for _, scope := range p.scopes {
		maps.Copy(ids, scope.ids)
	}
	for _, change := range p.Changes {
		for _, hunk := range change.Hunks {
			for _, i := range hunk.Removed {
				addScopeIDs(ids, p.baseline[change.Name].lines[i])
			}
			for _, i := range hunk.Added {
				addScopeIDs(ids, p.current[change.Name].lines[i])
			}
		}
	}
	return ids
}

func addScopeIDs(ids map[string]bool, text string) {
	for _, id := range scopeIDRe.FindAllString(text, -1) {
		ids[id] = true
	}
}

// boundaryUnits are the header and every unit of a risk-boundary section of
// a document: the contract terms a change is judged against, always carried.
func (p DeltaPlan) boundaryUnits(name string) []int {
	layout := p.current[name]
	units := []int{}
	if len(layout.lines) > 0 && layout.unitStart[0] == headerUnit {
		units = append(units, headerUnit)
	}
	for i := range layout.lines {
		if layout.unitStart[i] == i && riskBoundarySections[name][layout.h2[i]] {
			units = append(units, i)
		}
	}
	return units
}

// scopeUnits are the units of a document a reviewer needs to judge the open
// findings and what the changes may break: every unit whose label or text
// names a delta scope id, and every unit of a document or heading an
// unresolved finding's scope names. spec.md's traceability matrix
// contributes its matching rows instead (traceabilityRows).
func (p DeltaPlan) scopeUnits(name string) []int {
	layout := p.current[name]
	ids := idList(p.ids)
	var units []int
	for i := range layout.lines {
		if layout.unitStart[i] != i || name == "spec.md" && layout.h2[i] == traceabilitySection {
			continue
		}
		named := containsAnyID(layout.unitText(i), ids)
		for _, scope := range p.scopes {
			named = named || scope.docs[name] || scope.namesHeading(layout, i)
		}
		if named {
			units = append(units, i)
		}
	}
	return units
}

// traceabilityRows are the spec.md traceability rows naming a delta scope id.
func (p DeltaPlan) traceabilityRows() []string {
	layout := p.current["spec.md"]
	var rows []string
	for i, line := range layout.lines {
		if layout.h2[i] == traceabilitySection && strings.HasPrefix(strings.TrimSpace(line), "|") && containsAnyID(line, idList(p.ids)) {
			rows = append(rows, line)
		}
	}
	return rows
}

// requiredUnits are every unit of a document the request must carry in
// full: the units its changes touch, its boundary units, and its scope units.
func (p DeltaPlan) requiredUnits(name string) []int {
	units := append(p.boundaryUnits(name), p.scopeUnits(name)...)
	for _, change := range p.Changes {
		if change.Name == name {
			units = append(units, p.affectedUnits(change)...)
		}
	}
	slices.Sort(units)
	return slices.Compact(units)
}

// unitBody is the full text of a unit, the header included.
func (l docLayout) unitBody(start int) string {
	if start != headerUnit {
		return l.unitText(start)
	}
	end := 0
	for end < len(l.lines) && l.unitStart[end] == headerUnit {
		end++
	}
	return strings.TrimRight(strings.Join(l.lines[:end], "\n"), "\n") + "\n"
}

// unitName labels a unit in the request.
func (l docLayout) unitName(start int) string {
	if start == headerUnit {
		return "(header)"
	}
	return l.unitLabel(start)
}

// sortedIDs lists the delta scope ids for the request.
func (p DeltaPlan) sortedIDs() []string {
	return slices.Sorted(maps.Keys(p.ids))
}
