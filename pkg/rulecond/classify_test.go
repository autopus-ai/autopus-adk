package rulecond_test

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// TestClassify_TotalAndMutuallyExclusive covers REQ-CONDRULE-SCHEMA-02 and the
// INV-002 partition: every rule lands in exactly one class.
func TestClassify_TotalAndMutuallyExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		condition   string
		scope       string
		globs       []string
		skillScoped bool
		want        rulecond.Class
	}{
		{name: "no trigger field", want: rulecond.ClassAlways},
		{name: "globs only", globs: []string{"**/*.go"}, want: rulecond.ClassPathsScoped},
		{
			name:      "condition with tool scope",
			condition: `\bgit\s+commit\b`,
			scope:     "tool:bash",
			want:      rulecond.ClassHookFired,
		},
		{
			name:      "condition with tool scope wins over globs",
			condition: `\.go$`,
			scope:     "tool:edit(**/*.go)",
			globs:     []string{"**/*.go"},
			want:      rulecond.ClassHookFired,
		},
		{name: "skillScoped only", skillScoped: true, want: rulecond.ClassSkillScoped},
		{
			name:        "condition with tool scope wins over skillScoped",
			condition:   `\bgit\s+commit\b`,
			scope:       "tool:bash",
			skillScoped: true,
			want:        rulecond.ClassHookFired,
		},
		{
			name:        "skillScoped wins over globs",
			globs:       []string{"**/*.go"},
			skillScoped: true,
			want:        rulecond.ClassSkillScoped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rule := &rulecond.Rule{
				Name: "sample", SourceFile: "sample.md",
				Globs: tt.globs, SkillScoped: tt.skillScoped,
			}
			if tt.condition != "" {
				rule.Conditions = []string{tt.condition}
			}
			if tt.scope != "" {
				rule.Scopes = []string{tt.scope}
			}
			assert.Equal(t, tt.want, rulecond.Classify(rule))
		})
	}
}

// TestParseScope_ToolScopes covers the T2 scope grammar.
func TestParseScope_ToolScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope   string
		want    rulecond.Trigger
		wantErr bool
	}{
		{scope: "tool:bash", want: rulecond.Trigger{Event: preToolUse, Matcher: bashMatcher}},
		{scope: "tool:edit(*.go)", want: rulecond.Trigger{Event: preToolUse, Matcher: editMatcher, Glob: "*.go"}},
		{scope: "tool:write(**/*.md)", want: rulecond.Trigger{Event: preToolUse, Matcher: editMatcher, Glob: "**/*.md"}},
		{scope: "file:**/*.go", wantErr: true},
		{scope: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			t.Parallel()
			got, err := rulecond.ParseScope(tt.scope)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConditionSubject_PerTool is the INV-007 oracle for REQ-CONDRULE-FIRE-02.
func TestConditionSubject_PerTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tool    string
		input   map[string]any
		want    string
		wantAny bool
	}{
		{name: "bash uses command", tool: "Bash", input: map[string]any{"command": "git gc"}, want: "git gc", wantAny: true},
		{name: "edit uses file_path", tool: "Edit", input: map[string]any{"file_path": "/repo/a.go"}, want: "/repo/a.go", wantAny: true},
		{name: "write uses file_path", tool: "Write", input: map[string]any{"file_path": "/repo/b.go"}, want: "/repo/b.go", wantAny: true},
		{name: "multiedit uses file_path", tool: "MultiEdit", input: map[string]any{"file_path": "/repo/c.go"}, want: "/repo/c.go", wantAny: true},
		{name: "read has no subject", tool: "Read", input: map[string]any{"file_path": "/repo/d.go"}},
		{name: "task has no subject", tool: "Task", input: map[string]any{"command": "git commit"}},
		{name: "bash without command", tool: "Bash", input: map[string]any{"file_path": "/repo/e.go"}},
		{name: "edit without file_path", tool: "Edit", input: map[string]any{"command": "git commit"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := rulecond.ConditionSubject(tt.tool, tt.input)
			assert.Equal(t, tt.wantAny, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestClassify_ShippedRules is the REQ-CONDRULE-MAP-01 and REQ-CONDRULE-MAP-02
// oracle: the exact per-rule classification of the 3 rules the verification
// harness ships, one per class it still uses.
func TestClassify_ShippedRules(t *testing.T) {
	t.Parallel()

	want := map[string]rulecond.Class{
		"lore-commit":     rulecond.ClassHookFired,
		"file-size-limit": rulecond.ClassPathsScoped,
		"language-policy": rulecond.ClassAlways,
	}

	entries, err := fs.ReadDir(contentfs.FS, "rules")
	require.NoError(t, err)

	got := map[string]rulecond.Class{}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, readErr := fs.ReadFile(contentfs.FS, "rules/"+entry.Name())
		require.NoError(t, readErr)

		rule, parseErr := rulecond.ParseRule(entry.Name(), raw)
		require.NoError(t, parseErr, entry.Name())
		require.NoError(t, rulecond.Validate(rule), entry.Name())

		name := strings.TrimSuffix(entry.Name(), ".md")
		names = append(names, name)
		got[name] = rulecond.Classify(rule)
	}

	sort.Strings(names)
	require.Len(t, names, 3, "content/rules must hold the 3 shipped rules")
	assert.Equal(t, want, got)
}

// TestClassify_ShippedHookFiredConditions pins the T7 condition assignment.
func TestClassify_ShippedHookFiredConditions(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"lore-commit":       condLoreCommit,
		"shell-portability": condShellPortable,
		"worktree-safety":   condWorktreeSafety,
	}

	for name, condition := range want {
		raw := []byte(ruleSourceText(t, name))
		rule, err := rulecond.ParseRule(name+".md", raw)
		require.NoError(t, err)

		assert.Equal(t, []string{condition}, rule.Conditions, name)
		assert.Equal(t, []string{"tool:bash"}, rule.Scopes, name)
	}
}
