package cli

import (
	"fmt"
	"io"
)

// Run output of the local_patch claims (SPEC-SIGMABAND-002 BS Record, run
// output): for every local_patch claim that this run ended, the text output
// and the JSON envelope's local_patches[] show the result status, the patch
// file, the changed paths with their git apply --numstat counts, the
// requested and actual model of the patch request, and the warning. A run
// without such a claim prints and encodes exactly SPEC-SIGMABAND-001's
// report. Every value is a code, a model name of the models[] alphabet, a
// path under <lp>, or a path the Patch Policy accepted, so none can steer a
// terminal.

// bandRunData is the JSON data of a band run.
type bandRunData struct {
	bandReport
	LocalPatches []bandLocalPatchReport `json:"local_patches,omitempty"`
}

// printBandLocalPatches writes one block per local_patch claim after the
// series rows.
func printBandLocalPatches(w io.Writer, patches []bandLocalPatchReport) {
	for _, patch := range patches {
		line := "local patch " + patch.ClaimID + " " + patch.Status
		if patch.PatchPath != "" {
			line += " patch=" + patch.PatchPath
		}
		fmt.Fprintln(w, line)
		for _, file := range patch.Files {
			fmt.Fprintf(w, "  file %s +%d -%d\n", file.Path, file.Added, file.Removed)
		}
		if patch.RequestedModel != "" || patch.ActualModel != "" {
			fmt.Fprintf(w, "  model requested=%s actual=%s\n", defaultString(patch.RequestedModel, "-"), defaultString(patch.ActualModel, "-"))
		}
		fmt.Fprintln(w, "  warning: "+patch.Warning)
	}
}
