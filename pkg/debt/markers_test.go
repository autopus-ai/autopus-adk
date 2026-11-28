package debt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarkers_OnlyRealComments(t *testing.T) {
	cases := []struct {
		file, line string
		want       bool
	}{
		{"a.go", "x := 1 // TODO handle nil", true},
		{"a.go", "/* FIXME: leak */", true},
		{"a.go", " * HACK until v2", true},
		{"a.py", "# XXX: slow path", true},
		{"q.sql", "-- TODO index this", true},
		{"a.go", `s := "// TODO not a comment"`, false},
		{"a.go", "u := \"https://XXX.example.com\"", false},
		{"a.go", "u := `## TODO inside raw string`", false},
		{"q.sql", "SELECT * FROM t WHERE name LIKE '%TODO%'", false},
		{"run.sh", "git checkout -- TODO.md", false},
		{"a.go", "msg := \"done; TODO later\"", false},
		{"a.go", "// TODO(#123) tracked", false},
		{"a.go", "// TODO see https://github.com/x/y/issues/9", false},
		{"a.go", "// TODO keep UTF-8 output", true},
		{"a.go", "// TODO JIRA-42 tracked", false},
		{"a.go", "// TODO(ABC-7) tracked", false},
		{"a.go", "// TODO: drop the IE-11 workaround", true},
		{"a.go", "// TODO: ship before Q3-2027", true},
		{"a.go", `println("can't open") // TODO handle`, true},
		{"a.py", `print("user's file")  # TODO close it`, true},
		{"run.sh", `echo "don't" # HACK for now`, true},
		{"v.m", "// TODO objc", true},
		{"v.m", "% TODO matlab", true},
		{"a.ml", "(* TODO ocaml *)", true},
	}
	for _, tc := range cases {
		got := len(markers(tc.file, []string{tc.line})) == 1
		assert.Equal(t, tc.want, got, "%s: %s", tc.file, tc.line)
	}
}

func TestMarkers_KeyIgnoresFileAndIsValidUTF8(t *testing.T) {
	a := markers("old/a.go", []string{"// TODO move me"})
	b := markers("new/b.go", []string{"x := 1", "// TODO move me"})
	assert.Equal(t, a[0].Key, b[0].Key, "a moved file keeps its markers")
	bad := markers("cp949.go", []string{"// TODO \xc7\xd1\xb1\xdb"})
	assert.True(t, strings.ToValidUTF8(bad[0].Key, "") == bad[0].Key)
}

func TestCodeLines_SkipImports(t *testing.T) {
	text, _ := codeLines(strings.Split("package a\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\nimport \"strings\"\nfrom x import y\nuse std::io;\n#include <stdio.h>\nconst fs = require('fs')\nfunc A() { fmt.Println(os.Args) }\n", "\n"))
	assert.Equal(t, []string{"func A() { fmt.Println(os.Args) }"}, text)
}
