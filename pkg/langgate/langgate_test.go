package langgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheck(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		lang, text string
		want       bool
	}{
		"english status line":       {"ko", "Waiting for the internal/cli result while the full suite runs in the background.", false},
		"english paragraph":         {"ko", "Both remaining items are fixed. The loop now restores scenarios from the newest loop branch.", false},
		"korean with english terms": {"ko", "loop가 고친 fix commit은 autopus/qa-loop branch에 남고, rerun은 scenario file만 restore합니다.", true},
		"korean report with code":   {"ko", "남은 두 가지를 고쳤습니다.\n```bash\ngo test -timeout 40m ./internal/cli ./pkg/qa/... && echo all good here\n```\n`TestRun_RerunRestoresScenariosFromTheLoopBranch`가 통과합니다.", true},
		"short english":             {"ko", "Merged.", true},
		"only code and url":         {"ko", "```\nfunc main() { fmt.Println(\"hello world from the generated binary\") }\n```\nhttps://github.com/autopus-ai/autopus-adk/pull/283", true},
		"english config":            {"en", "Waiting for the internal/cli result while the full suite runs in the background.", true},
		"unset language":            {"", "Waiting for the internal/cli result while the full suite runs in the background.", true},
		"japanese":                  {"ja", "テストはすべて通りました。internal/cli の結果を待っています。", true},
		"english under ja":          {"ja-JP", "Waiting for the internal/cli result while the full suite runs in the background.", false},
	} {
		assert.Equal(t, tc.want, Check(tc.lang, tc.text), name)
	}
}

func TestSupportedAndName(t *testing.T) {
	t.Parallel()
	assert.True(t, Supported("KO"))
	assert.False(t, Supported("en"))
	assert.Equal(t, "Korean", Name("ko"))
	assert.Equal(t, "fr", Name("fr"))
}
