package verifygate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// goTestEvent is the part of a `go test -json` event the gate reads.
type goTestEvent struct {
	Action     string
	Package    string
	ImportPath string
	Test       string
	Output     string
}

// renderGoTestJSON turns `go test -json` output back into the text a plain
// `go test` prints on failure: package-level lines and the output of failed
// tests, without the -v chatter of passing ones. It also returns the failed
// tests as "pkg.Test" in order, or the bare package when it failed with no
// failed test (a build failure, TestMain, os.Exit), so a run cut short by
// the budget still reports what had already failed. Lines that are not JSON events (build
// errors on stderr) are kept as they are.
func renderGoTestJSON(out []byte) (string, []string) {
	var text strings.Builder
	var failed []string
	pending := map[string]*strings.Builder{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var ev goTestEvent
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			text.Write(line)
			text.WriteByte('\n')
			continue
		}
		if ev.Test == "" {
			text.WriteString(ev.Output)
			// A package fails without a failed test when its tests do not
			// compile, TestMain fails, or a test calls os.Exit.
			pkg := ev.Package
			if pkg == "" {
				// build-fail names the test binary: "m/b [m/b.test]".
				pkg, _, _ = strings.Cut(ev.ImportPath, " ")
			}
			if (ev.Action == "fail" || ev.Action == "build-fail") && pkg != "" &&
				!slices.Contains(failed, pkg) &&
				!slices.ContainsFunc(failed, func(k string) bool { return strings.HasPrefix(k, pkg+".") }) {
				failed = append(failed, pkg)
			}
			continue
		}
		key := ev.Package + "." + ev.Test
		switch ev.Action {
		case "output":
			b := pending[key]
			if b == nil {
				b = &strings.Builder{}
				pending[key] = b
			}
			b.WriteString(ev.Output)
		case "fail":
			if b := pending[key]; b != nil {
				text.WriteString(b.String())
			}
			delete(pending, key)
			if !slices.Contains(failed, key) {
				failed = append(failed, key)
			}
		case "pass", "skip":
			delete(pending, key)
		}
	}
	return text.String(), failed
}
