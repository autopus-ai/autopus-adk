package cli

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// workflowsRel is the workflow directory relative to the repository root.
const workflowsRel = ".github/workflows"

// workflowSpecs limit git to the files the lint reads: YAML directly in the
// workflow directory, so a stray .DS_Store or a subdirectory is no edit.
var workflowSpecs = []string{":(glob)" + workflowsRel + "/*.yml", ":(glob)" + workflowsRel + "/*.yaml"}

// remoteWorkflows decides which workflow files the lint reads when no
// --workflows-dir was given. The runs come from GitHub, so a checkout that
// is only behind its remote default branch would lint files that no longer
// run there: in that case it returns the remote branch's files. A checkout
// whose own commits or uncommitted edits touch the workflows keeps the local
// files, with a note when they differ from the remote. It returns nil files
// when the local checkout should be linted, and an empty non-nil map when the
// remote default branch has no workflow files, so there is nothing to lint.
//
// The command is documented as read-only, so every git call here leaves
// .git/index alone: a diff against the work tree and a plain status both
// refresh and rewrite the index under its lock, which fails a concurrent
// `git commit` in the same repository.
func remoteWorkflows(top string) (map[string][]byte, string) {
	ref, err := ciGit(top, "rev-parse", "--abbrev-ref", "origin/HEAD")
	if err != nil || ref == "" || ref == "origin/HEAD" {
		if _, err := ciGit(top, "remote", "get-url", "origin"); err != nil {
			if remotes, _ := ciGit(top, "remote"); remotes == "" {
				return nil, ""
			}
			return nil, "the repository has no remote named origin, so the workflow lint read the local " +
				"checkout without comparing it to the remote default branch"
		}
		return nil, "origin/HEAD does not resolve, so the workflow lint read the local checkout " +
			"without comparing it to the remote default branch; `git remote set-head origin --auto` " +
			"(or `git fetch origin` on git 2.48+) fixes this"
	}
	// HEAD against ref compares two commits and never touches the index; the
	// status below covers the work tree.
	changed, err := ciGit(top, append([]string{"diff", "--name-only", "HEAD", ref, "--"}, workflowSpecs...)...)
	if err != nil {
		return nil, ""
	}
	dirty, errD := ciGit(top, append([]string{"--no-optional-locks", "status", "--porcelain",
		"--untracked-files=all", "--"}, workflowSpecs...)...)
	if changed == "" && (errD != nil || dirty == "") {
		return nil, ""
	}
	// Only local commits that touch the workflows make the checkout an edit
	// in progress. Counting commits instead kept every feature branch, and
	// every squash-merged [gone] branch, on its stale workflow files.
	localEdits, errL := ciGit(top, append([]string{"diff", "--name-only", ref + "...HEAD", "--"}, workflowSpecs...)...)
	behind, errB := ciGit(top, "rev-list", "--count", "HEAD.."+ref)
	if errL != nil || errB != nil || errD != nil || localEdits != "" || dirty != "" {
		return nil, fmt.Sprintf("the workflow lint read the local %s, which differs from %s; "+
			"findings may not match the workflows that ran", workflowsRel, ref)
	}
	files, err := gitWorkflowFiles(top, ref)
	if err != nil {
		return nil, fmt.Sprintf("the local %s is %s commits behind %s and its files could not be read; "+
			"the lint read the stale checkout", workflowsRel, behind, ref)
	}
	if len(files) == 0 {
		return map[string][]byte{}, fmt.Sprintf("%s has no workflow files in %s, so the workflow lint was skipped",
			ref, workflowsRel)
	}
	return files, fmt.Sprintf("the checkout is %s commits behind %s, so the workflow lint read %s's %s",
		behind, ref, ref, workflowsRel)
}

// gitWorkflowFiles reads the *.yml and *.yaml files directly in the workflow
// directory of ref, bounded like cireport.LoadWorkflows.
func gitWorkflowFiles(top, ref string) (map[string][]byte, error) {
	// -z keeps paths unquoted, so non-ASCII and quoted names survive.
	list, err := ciGit(top, "ls-tree", "-z", "-l", ref, workflowsRel+"/")
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, entry := range strings.Split(list, "\x00") {
		// "<mode> blob <sha> <size>\t<path>"
		meta, p, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		// A symlink is a blob too; LoadWorkflows reads regular files only.
		if !ok || len(fields) != 4 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			continue
		}
		if ext := path.Ext(p); ext != ".yml" && ext != ".yaml" {
			continue
		}
		if size, err := strconv.Atoi(fields[3]); err != nil || size > cireport.MaxWorkflowBytes {
			continue
		}
		data, err := ciGit(top, "show", ref+":"+p)
		if err != nil {
			return nil, err
		}
		files[path.Base(p)] = []byte(data)
	}
	return files, nil
}
