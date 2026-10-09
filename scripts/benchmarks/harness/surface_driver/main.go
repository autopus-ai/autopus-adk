// Command surface_driver writes the default generated harness surface of the
// five platforms for one source revision (SPEC-HARNEVAL-001 REQ-HE-07, T13).
//
// The golden live runner extracts an arm revision with git archive, installs
// this file as the only file of scripts/benchmarks/harness/surface_driver in
// that tree, and builds it there with the pinned generator version linked in:
//
//	go build -trimpath \
//	  -ldflags "-X github.com/insajin/autopus-adk/pkg/version.version=<pins.generator_version>" \
//	  ./scripts/benchmarks/harness/surface_driver
//
// It uses only API that v0.50.122 already has, so a previous release tag
// builds it too: config.DefaultFullConfig, config.Save, each platform's
// NewWithRoot, the codex and opencode host-probe pins, Generate, and
// version.Version. It mirrors harneval.Generate for the default
// configuration. The linked-in version stands in for the in-process
// codex.WithPluginBaseVersion pin, so the driver refuses to run when the link
// did not take: a revision must never fall back to its build version.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/version"
)

// platforms is the generation order of harneval.PinnedAdapters.
var platforms = []string{"claude-code", "codex", "antigravity-cli", "opencode", "omp"}

// generator is the part of a platform adapter the driver calls.
type generator interface {
	Name() string
	Generate(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error)
}

// options are the candidate manifest pins the runner passes. The generator
// version reaches generation through the linker; the flag only checks it.
type options struct {
	output, projectName, generatorVersion             string
	codexCatalog, codexCLIVersion, opencodeCLIVersion string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "surface_driver: "+err.Error())
		os.Exit(1)
	}
}

func parse(args []string) (options, error) {
	var o options
	flags := flag.NewFlagSet("surface_driver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&o.output, "output", "", "new directory for the generated surface")
	flags.StringVar(&o.projectName, "project-name", "", "pins.project_name")
	flags.StringVar(&o.generatorVersion, "generator-version", "", "pins.generator_version; must equal the linked-in version")
	flags.StringVar(&o.codexCatalog, "codex-model-catalog", "", "file holding the pinned codex model catalog; empty pins no catalog")
	flags.StringVar(&o.codexCLIVersion, "codex-cli-version", "", "pins.codex_cli_version")
	flags.StringVar(&o.opencodeCLIVersion, "opencode-cli-version", "", "pins.opencode_cli_version")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	switch {
	case flags.NArg() > 0:
		return o, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	case o.output == "" || o.projectName == "" || o.generatorVersion == "" || o.codexCLIVersion == "" || o.opencodeCLIVersion == "":
		return o, errors.New("--output, --project-name, --generator-version, --codex-cli-version and --opencode-cli-version are required")
	case version.Version() != o.generatorVersion:
		return o, fmt.Errorf("linked-in version %q is not the pinned generator version %q: build with "+
			"-ldflags \"-X github.com/insajin/autopus-adk/pkg/version.version=%s\"", version.Version(), o.generatorVersion, o.generatorVersion)
	}
	return o, nil
}

// run generates the surface into a new --output directory: the default full
// configuration for every platform, saved first, then each platform adapter
// in order with every known host probe answered from the pins.
func run(args []string, stdout io.Writer) error {
	o, err := parse(args)
	if err != nil {
		return err
	}
	var catalog []byte
	if o.codexCatalog != "" {
		if catalog, err = os.ReadFile(o.codexCatalog); err != nil {
			return fmt.Errorf("codex model catalog: %w", err)
		}
	}
	if err := os.Mkdir(o.output, 0o755); err != nil {
		return fmt.Errorf("output: %w", err)
	}
	cfg := config.DefaultFullConfig(o.projectName)
	cfg.Platforms = append([]string(nil), platforms...)
	if err := config.Save(o.output, cfg); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	// Like harneval.Generate, stand for a repository root: adapters write
	// root-local git hooks only into a gitdir holding HEAD.
	if err := os.MkdirAll(filepath.Join(o.output, ".git"), 0o755); err != nil {
		return fmt.Errorf("gitdir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(o.output, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		return fmt.Errorf("gitdir: %w", err)
	}
	ctx := context.Background()
	for _, g := range generators(o, catalog) {
		if _, err := g.Generate(ctx, cfg); err != nil {
			return fmt.Errorf("generate %s: %w", g.Name(), err)
		}
	}
	return json.NewEncoder(stdout).Encode(map[string]any{"generator_version": version.Version(), "platforms": platforms})
}

func generators(o options, catalog []byte) []generator {
	return []generator{
		claude.NewWithRoot(o.output),
		codex.NewWithRoot(o.output, codex.WithModelCatalog(catalog), codex.WithCLIVersion(o.codexCLIVersion)),
		antigravity.NewWithRoot(o.output),
		opencode.NewWithRoot(o.output, opencode.WithCLIVersion(o.opencodeCLIVersion)),
		omp.NewWithRoot(o.output),
	}
}
