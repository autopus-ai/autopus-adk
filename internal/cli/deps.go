package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/freshness"
)

// depsEcosystems maps the accepted `<ecosystem>:` prefixes, aliases included.
var depsEcosystems = map[string]freshness.Ecosystem{
	"go": freshness.EcoGo, "npm": freshness.EcoNPM, "pypi": freshness.EcoPyPI, "pip": freshness.EcoPyPI,
	"crates": freshness.EcoCrates, "cargo": freshness.EcoCrates, "image": freshness.EcoImage, "docker": freshness.EcoImage,
	"toolchain": freshness.EcoToolchain,
}

func newDepsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Look up latest stable versions of dependencies, toolchains and container images",
		Long: "Asks the public sources (proxy.golang.org, registry.npmjs.org, pypi.org, crates.io, Docker Hub, go.dev, " +
			"nodejs.org, endoflife.date) for the latest stable release, excluding pre-releases. Private Go modules " +
			"(GOPRIVATE, GONOPROXY, GONOSUMDB) and private registries are never sent to a public source. Answers are " +
			"cached for 24h; AUTOPUS_DEPS_OFFLINE=1 answers from the cache only.",
	}
	cmd.AddCommand(newDepsLatestCmd(), newDepsOutdatedCmd())
	return cmd
}

func newDepsLatestCmd() *cobra.Command {
	var (
		dir     string
		asJSON  bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "latest <ecosystem>:<name> | <name>",
		Short: "Print the latest stable version of one package, toolchain or image",
		Long: "Ecosystems: go, npm, pypi, crates, image, toolchain (toolchain:go, toolchain:node, toolchain:python). " +
			"A bare name is looked up in the project's manifests to find its ecosystem; go, node and python alone " +
			"mean the toolchain. For Node the active LTS line is printed too: use it for engines, .nvmrc and images.",
		Example: "  auto deps latest npm:react\n  auto deps latest image:postgres\n  auto deps latest toolchain:node\n  auto deps latest django",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := resolveDepsItem(args[0], dir)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			latest := newFreshnessClient().Lookup(ctx, item)
			if asJSON {
				return writeDepsJSON(cmd.OutOrStdout(), map[string]any{"item": item, "latest": latest})
			}
			if latest.Err != "" {
				return fmt.Errorf("%s: latest version unknown: %s", depsLabel(item), latest.Err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s %s\n", depsLabel(item), latest.Version)
			if latest.LTS != "" {
				fmt.Fprintf(out, "  lts      %s (active LTS: use this line for engines, .nvmrc and images)\n", latest.LTS)
			}
			fmt.Fprintf(out, "  source   %s\n  fetched  %s\n", latest.Source, latest.FetchedAt.UTC().Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory used to resolve a bare name")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	cmd.Flags().DurationVar(&timeout, "timeout", 15*time.Second, "Overall lookup timeout")
	return cmd
}

// resolveDepsItem reads `<ecosystem>:<name>`, or finds a bare name in the
// project's manifests.
func resolveDepsItem(arg, dir string) (freshness.Item, error) {
	if prefix, name, ok := strings.Cut(arg, ":"); ok {
		eco, known := depsEcosystems[strings.ToLower(prefix)]
		if !known {
			return freshness.Item{}, fmt.Errorf("unknown ecosystem %q: use go, npm, pypi, crates, image or toolchain", prefix)
		}
		if eco == freshness.EcoImage {
			if repo, _, tagged := strings.Cut(name, ":"); tagged {
				name = repo
			}
			name = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(name), "docker.io/"), "library/")
		}
		if eco == freshness.EcoToolchain && name != "go" && name != "node" && name != "python" {
			return freshness.Item{}, fmt.Errorf("unknown toolchain %q: use go, node or python", name)
		}
		return freshness.Item{Ecosystem: eco, Name: name}, nil
	}
	if items, err := freshness.Scan(dir); err == nil {
		for _, it := range items {
			if strings.EqualFold(it.Name, arg) {
				return freshness.Item{Ecosystem: it.Ecosystem, Name: it.Name, Registry: it.Registry}, nil
			}
		}
	}
	switch arg {
	case "go", "node", "python":
		return freshness.Item{Ecosystem: freshness.EcoToolchain, Name: arg}, nil
	}
	return freshness.Item{}, fmt.Errorf("%q is not in this project's manifests: name the ecosystem, e.g. npm:%s or pypi:%s", arg, arg, arg)
}

func newDepsOutdatedCmd() *cobra.Command {
	var (
		dir     string
		asJSON  bool
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "Compare the project's direct dependencies, toolchain pins and images with the latest stable",
		Long: "Reads go.mod, package.json, requirements*.txt, pyproject.toml, Cargo.toml, .nvmrc, .node-version, " +
			".python-version, .tool-versions, Dockerfiles and YAML image: keys (docker-compose, Kubernetes, CI services). " +
			"Each pin is classified as current, minor-behind, major-behind (a whole release line), unknown (the source " +
			"could not be asked) or skipped. Node runtime pins are compared with the active LTS line. Always exits 0.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := freshness.Scan(dir)
			if err != nil {
				return err
			}
			sort.SliceStable(items, func(a, b int) bool {
				if items[a].File != items[b].File {
					return items[a].File < items[b].File
				}
				return items[a].Line < items[b].Line
			})
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			latests := newFreshnessClient().LookupAll(ctx, items)
			results := make([]freshness.Result, len(items))
			for i, it := range items {
				results[i] = freshness.Classify(it, latests[i])
			}
			if asJSON {
				return writeDepsJSON(cmd.OutOrStdout(), map[string]any{"dir": dir, "results": results})
			}
			writeOutdatedTable(cmd.OutOrStdout(), results)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "Project directory to scan")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Overall lookup timeout")
	return cmd
}

func writeOutdatedTable(out io.Writer, results []freshness.Result) {
	if len(results) == 0 {
		fmt.Fprintln(out, "no dependency, toolchain or image pins found")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FILE\tITEM\tPINNED\tLATEST\tSTATUS")
	counts := map[freshness.Status]int{}
	for _, r := range results {
		counts[r.Status]++
		status := string(r.Status)
		switch {
		case r.LinesBehind > 0:
			status = fmt.Sprintf("%s (%d)", status, r.LinesBehind)
		case r.Reason != "" && r.Status != freshness.StatusCurrent:
			status += ": " + r.Reason
		}
		latest := r.Target
		if r.Latest.LTS != "" && r.Latest.Version != r.Latest.LTS {
			latest += " (LTS; current " + r.Latest.Version + ")"
		}
		fmt.Fprintf(tw, "%s:%d\t%s\t%s\t%s\t%s\n", r.Item.File, r.Item.Line, depsLabel(r.Item), r.Item.Spec, latest, status)
	}
	_ = tw.Flush()
	fmt.Fprintf(out, "\n%d pins: %d major-behind, %d minor-behind, %d current, %d unknown, %d skipped\n", len(results),
		counts[freshness.StatusMajor], counts[freshness.StatusMinor], counts[freshness.StatusCurrent],
		counts[freshness.StatusUnknown], counts[freshness.StatusSkipped])
}

func depsLabel(it freshness.Item) string { return string(it.Ecosystem) + ":" + it.Name }

func writeDepsJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
