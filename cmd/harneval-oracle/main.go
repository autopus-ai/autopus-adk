// Command harneval-oracle is the trusted black-box oracle harness of the
// SPEC-HARNEVAL-003 signed live lane (REQ-HR-08).
//
// The trusted runner (scripts/benchmarks/harness/golden_blackbox.py) builds it
// from the runner checkout and starts it as a sibling process under oracle.sb,
// only after the agent-modified artifact and every process it left have ended:
//
//	harneval-oracle --task <id> --outputs <artifact output root> --result <dir> < bundle
//
// The bundle on stdin (harness_oracle_input.v1) holds everything the judgement
// needs besides the output files: the assertion definitions and fixed output
// paths from the main task definition, the expected outputs the runner read
// and checked against their pinned SHA-256, the captured stdout and the exit
// status. The harness never reads an expected output from disk and takes no
// path from the artifact's output: it opens only the fixed relative paths the
// bundle names, below the output root, as regular single-link files without
// following links. It writes one harness_oracle_result.v1 document,
// <dir>/oracle_result.json, and exits 0; any other exit leaves no result.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ResultFile is the name of the result document inside the --result directory.
const ResultFile = "oracle_result.json"

// maxInput caps the stdin bundle: at most maxAssertions expected outputs and
// one stdout, each at most OutputLimit bytes, in base64.
const maxInput = 64 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

// run judges one artifact run and returns the process exit code: 0 when the
// result document was written, 1 when the arguments, the bundle or the result
// directory were unusable, and no result exists.
func run(args []string, stdin io.Reader, stderr io.Writer) int {
	flags := flag.NewFlagSet("harneval-oracle", flag.ContinueOnError)
	flags.SetOutput(stderr)
	task := flags.String("task", "", "task id the bundle must name")
	outputs := flags.String("outputs", "", "canonical artifact output root")
	resultDir := flags.String("result", "", "existing directory for "+ResultFile)
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if flags.NArg() != 0 || *task == "" || !filepath.IsAbs(*outputs) || !filepath.IsAbs(*resultDir) {
		fmt.Fprintln(stderr, "harneval-oracle: --task, an absolute --outputs and an absolute --result are required")
		return 1
	}
	data, err := io.ReadAll(io.LimitReader(stdin, maxInput+1))
	if err == nil && len(data) > maxInput {
		err = fmt.Errorf("bundle exceeds %d bytes", maxInput)
	}
	var bundle input
	if err == nil {
		bundle, err = decodeInput(data, *task)
	}
	if err != nil {
		fmt.Fprintln(stderr, "harneval-oracle: invalid bundle:", err)
		return 1
	}
	if err := writeResult(filepath.Join(*resultDir, ResultFile), judge(bundle, *outputs)); err != nil {
		fmt.Fprintln(stderr, "harneval-oracle: result:", err)
		return 1
	}
	return 0
}

// decodeInput strictly decodes one harness_oracle_input.v1 document: no unknown
// field and no data after it.
func decodeInput(data []byte, task string) (input, error) {
	var bundle input
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return bundle, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return bundle, errors.New("data follows the bundle")
	}
	return bundle, bundle.validate(task)
}

// writeResult creates the result document exclusively with mode 0600; a
// partly written file is removed.
func writeResult(path string, document result) error {
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}
