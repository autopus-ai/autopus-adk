// Package content_test는 방법론 설정 검증 게이트의 테스트이다.
package content_test

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/content"
)

// methodologyFS는 주어진 YAML 본문을 methodology/<mode>.yaml로 노출한다.
func methodologyFS(mode, body string) fstest.MapFS {
	return fstest.MapFS{
		"methodology/" + mode + ".yaml": &fstest.MapFile{Data: []byte(body)},
	}
}

// TestResolveEnforcedMethodology_Defects는 enforce: true에서 게이트가
// fail-closed임을 결함 종류별로 고정한다. 각 실패 메시지는 원인을 이름으로 지목한다.
func TestResolveEnforcedMethodology_Defects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mode        string
		fsys        fstest.MapFS
		wantDefect  content.MethodologyDefect
		wantMessage []string
	}{
		{
			name:        "unknown mode",
			mode:        "kanban",
			fsys:        methodologyFS("tdd", "name: tdd\nstages:\n  - name: red\n"),
			wantDefect:  content.DefectUnknownMode,
			wantMessage: []string{"kanban"},
		},
		{
			name: "cycle",
			mode: "loop",
			fsys: methodologyFS("loop", `name: loop
stages:
  - name: red
    required_before: refactor
  - name: green
    required_before: red
  - name: refactor
    required_before: green
`),
			wantDefect:  content.DefectCycle,
			wantMessage: []string{"red", "순환"},
		},
		{
			name: "dangling required_before",
			mode: "gap",
			fsys: methodologyFS("gap", `name: gap
stages:
  - name: red
    required_before: ""
  - name: green
    required_before: crimson
`),
			wantDefect:  content.DefectDanglingRequirement,
			wantMessage: []string{"green", "crimson"},
		},
		{
			name: "duplicate stage name",
			mode: "twin",
			fsys: methodologyFS("twin", `name: twin
stages:
  - name: red
    required_before: ""
  - name: red
    required_before: red
`),
			wantDefect:  content.DefectDuplicateStage,
			wantMessage: []string{"red", "중복"},
		},
		{
			name: "multiple roots",
			mode: "forked",
			fsys: methodologyFS("forked", `name: forked
stages:
  - name: red
    required_before: ""
  - name: green
    required_before: ""
`),
			wantDefect:  content.DefectMultipleRoots,
			wantMessage: []string{"green"},
		},
		{
			name: "ambiguous order",
			mode: "branch",
			fsys: methodologyFS("branch", `name: branch
stages:
  - name: red
    required_before: ""
  - name: green
    required_before: red
  - name: blue
    required_before: red
`),
			wantDefect:  content.DefectAmbiguousOrder,
			wantMessage: []string{"green", "blue", "red"},
		},
		{
			name:        "no stages",
			mode:        "empty",
			fsys:        methodologyFS("empty", "name: empty\n"),
			wantDefect:  content.DefectNoStages,
			wantMessage: []string{"empty"},
		},
		{
			name: "empty stage name",
			mode: "nameless",
			fsys: methodologyFS("nameless", `name: nameless
stages:
  - name: ""
    description: 이름 없는 단계
`),
			wantDefect:  content.DefectEmptyStageName,
			wantMessage: []string{"nameless"},
		},
		{
			name:        "enforce without mode",
			mode:        "none",
			fsys:        methodologyFS("tdd", "name: tdd\nstages:\n  - name: red\n"),
			wantDefect:  content.DefectEnforceWithoutMode,
			wantMessage: []string{"enforce: true"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			def, err := content.ResolveEnforcedMethodology(tc.fsys, tc.mode, true)
			require.Error(t, err)
			assert.Nil(t, def)

			var defect *content.MethodologyError
			require.True(t, errors.As(err, &defect), "게이트 실패는 MethodologyError여야 한다: %v", err)
			assert.Equal(t, tc.wantDefect, defect.Defect)
			for _, fragment := range tc.wantMessage {
				assert.Contains(t, err.Error(), fragment)
			}
		})
	}
}

// TestResolveMethodology_NoneIsSilent는 mode: none이 아무것도 로드하지 않고
// 실패하지도 않음을 고정한다.
func TestResolveMethodology_NoneIsSilent(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"none", "", "  ", "NONE"} {
		def, err := content.ResolveMethodology(fstest.MapFS{}, mode)
		require.NoError(t, err)
		assert.Nil(t, def)
	}

	def, err := content.ResolveEnforcedMethodology(fstest.MapFS{}, "none", false)
	require.NoError(t, err)
	assert.Nil(t, def)
}

// TestResolveMethodology_OrdersStagesByRequiredBefore는 선언 순서가 아니라
// required_before 순서로 정렬됨을 고정한다.
func TestResolveMethodology_OrdersStagesByRequiredBefore(t *testing.T) {
	t.Parallel()

	fsys := methodologyFS("shuffled", `name: shuffled
stages:
  - name: refactor
    required_before: green
  - name: red
    required_before: ""
  - name: green
    required_before: red
`)

	def, err := content.ResolveMethodology(fsys, "shuffled")
	require.NoError(t, err)
	require.Len(t, def.Stages, 3)
	assert.Equal(t, []string{"red", "green", "refactor"},
		[]string{def.Stages[0].Name, def.Stages[1].Name, def.Stages[2].Name})
}

// TestResolveMethodology_PathTraversalRejected는 mode가 사용자 설정 값이므로
// 콘텐츠 트리 밖을 가리킬 수 없음을 고정한다.
func TestResolveMethodology_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"../secrets", "sub/tdd", ".."} {
		_, err := content.ResolveMethodology(contentfs.FS, mode)
		require.Error(t, err)

		var defect *content.MethodologyError
		require.True(t, errors.As(err, &defect))
		assert.Equal(t, content.DefectUnknownMode, defect.Defect)
	}
}

// TestResolveMethodology_ShippedDefinitions는 배포되는 세 정의가 게이트를
// 통과하고, 규칙 전문이 지침 텍스트로 전달됨을 고정한다.
func TestResolveMethodology_ShippedDefinitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode      string
		wantOrder []string
	}{
		{mode: "tdd", wantOrder: []string{"red", "green", "refactor"}},
		{mode: "ddd", wantOrder: []string{"analyze", "preserve", "improve"}},
		{mode: "double-diamond", wantOrder: []string{"discover", "define", "develop", "deliver"}},
	}

	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			t.Parallel()

			def, err := content.ResolveMethodology(contentfs.FS, tc.mode)
			require.NoError(t, err)
			require.NotNil(t, def)

			order := make([]string, 0, len(def.Stages))
			for _, stage := range def.Stages {
				order = append(order, stage.Name)
			}
			assert.Equal(t, tc.wantOrder, order)

			// The verification harness ships no methodology skill, so the
			// definition still validates but has no delivery surface.
			_, err = content.ResolveEnforcedMethodology(contentfs.FS, tc.mode, true)
			var defect *content.MethodologyError
			require.ErrorAs(t, err, &defect)
			assert.Equal(t, content.DefectMissingDelivery, defect.Defect)
		})
	}
}

// TestResolveMethodology_MissingDeliverySurfaceFails는 정의만 있고 전달 표면이
// 없으면 실패함을 고정한다. 그 조합은 아무도 읽지 않는 규칙을 선언하는 셈이다.
func TestResolveMethodology_MissingDeliverySurfaceFails(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"methodology/orphan.yaml": &fstest.MapFile{Data: []byte(
			"name: orphan\nstages:\n  - name: only\n    description: d\n    required_before: \"\"\n",
		)},
	}
	_, err := content.ResolveEnforcedMethodology(fsys, "orphan", false)
	require.Error(t, err)
	var defect *content.MethodologyError
	require.ErrorAs(t, err, &defect)
	assert.Equal(t, content.DefectMissingDelivery, defect.Defect)
	assert.Contains(t, err.Error(), "content/skills/orphan.md")
}
