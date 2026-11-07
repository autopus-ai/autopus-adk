package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextEngineering_ContextCatalogDoctorAndScenarioContract(t *testing.T) {
	t.Run("S6 ContextLoadSet identity is stable", func(t *testing.T) {
		expected := []contextLoadDoc{
			{Name: "product.md", RelPath: filepath.Join(".autopus", "project", "product.md"), Cap: 18000},
			{Name: "ARCHITECTURE.md", RelPath: "ARCHITECTURE.md", Cap: 16000},
			{Name: "scenarios.md", RelPath: filepath.Join(".autopus", "project", "scenarios.md"), Cap: 20000},
			{Name: "workspace.md", RelPath: filepath.Join(".autopus", "project", "workspace.md"), Cap: 12000},
			{Name: "tech.md", RelPath: filepath.Join(".autopus", "project", "tech.md"), Cap: 10000},
			{Name: "structure.md", RelPath: filepath.Join(".autopus", "project", "structure.md"), Cap: 18000},
			{Name: "canary.md", RelPath: filepath.Join(".autopus", "project", "canary.md"), Cap: 6000},
		}
		assert.Equal(t, expected, ContextLoadSet)
	})

	doctorCases := []struct {
		name        string
		sizes       map[string]int
		expectedIDs []string
		bytes       string
	}{
		{
			name:  "total cap",
			sizes: fixtureASizes, bytes: "130000",
			expectedIDs: []string{"doctor.context_weight.total"},
		},
		{
			name:  "per document cap",
			sizes: fixtureCSizes, bytes: "22000",
			expectedIDs: []string{"doctor.context_weight.total", "doctor.context_weight.doc.product.md"},
		},
	}
	for _, testCase := range doctorCases {
		testCase := testCase
		t.Run("S6 doctor "+testCase.name, func(t *testing.T) {
			dir := seedLoadSet(t, testCase.sizes)
			var output bytes.Buffer
			assert.True(t, checkContextWeight(&output, dir))
			assert.True(t, strings.Contains(strings.ToLower(output.String()), "context catalog"),
				"doctor text must use context catalog terminology")
			assert.Contains(t, output.String(), testCase.bytes)

			report := doctorJSONReport{status: jsonStatusOK}
			report.collectContextWeightChecks(dir)
			actualIDs := make([]string, 0, len(report.checks))
			for _, check := range report.checks {
				actualIDs = append(actualIDs, check.ID)
				assert.True(t, strings.Contains(strings.ToLower(check.Detail), "context catalog"),
					"doctor JSON detail must use context catalog terminology")
			}
			assert.Equal(t, testCase.expectedIDs, actualIDs)
			assert.Equal(t, jsonStatusOK, report.status, "context weight stays advisory")
		})
	}
}
