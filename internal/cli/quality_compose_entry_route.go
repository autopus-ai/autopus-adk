package cli

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityComposeRouter hands `auto quality` input from the composition
// prompt to the editors the target menu reaches (SPEC-MODELUX-001
// REQ-MUX-16), so the existing scripted inputs keep working: `global\n1\n`
// and `claude-code\n…` as before, and `t` opens the target menu itself.
type qualityComposeRouter struct {
	cmd   *cobra.Command
	dir   string
	cfg   *config.HarnessConfig
	apply bool
	deps  ompPlatformDependencies
}

// qualityTargetNames returns the targets chooseQualityTarget offers, in menu
// order: global, then every installed tool with per-agent models.
func qualityTargetNames(cfg *config.HarnessConfig) []string {
	names := []string{"global"}
	for _, tool := range configuredQualityModelTools(cfg) {
		names = append(names, tool.platform)
	}
	return names
}

// Targets lists every answer chooseQualityTarget accepts for a target: each
// name, then each menu number, so a script written for the target menu
// (`1\n2\n`) still reaches the same editor. A project without a model tool
// never had a target menu, so only `global` names its quality menu.
func (r qualityComposeRouter) Targets() []string {
	names := qualityTargetNames(r.cfg)
	if len(names) == 1 {
		return names
	}
	answers := slices.Clone(names)
	for index := range names {
		answers = append(answers, strconv.Itoa(index+1))
	}
	return answers
}

// Route runs target's editor as the target menu did once that target was
// chosen; the empty target is `t`, which opens the menu first. A tool editor
// reached by name first names the platforms without per-agent models, as the
// menu it skips would have. With --apply the editor generates with snapshot,
// the one the composition table read.
func (r qualityComposeRouter) Route(target string, snapshot driftGenerationSnapshot) error {
	apply := qualityEditorApply{enabled: r.apply, snapshot: snapshot}
	if target == "" {
		return runQualityTargetMenu(r.cmd, r.dir, r.cfg, apply, r.deps)
	}
	names := qualityTargetNames(r.cfg)
	if index, err := strconv.Atoi(target); err == nil && index >= 1 && index <= len(names) {
		target = names[index-1]
	}
	if target != "global" {
		writeInheritOnlyQualityToolNote(r.cmd.OutOrStdout(), r.cfg)
	}
	return runQualityTarget(r.cmd, r.dir, r.cfg, target, apply, r.deps)
}

// qualityEditorApply is the --apply of an editor the composition prompt
// hands input to. It regenerates with the snapshot the composition table
// read, so the run reads the Codex catalog once and generates what the table
// showed (REQ-MUX-19).
type qualityEditorApply struct {
	enabled  bool
	snapshot driftGenerationSnapshot
}

// run regenerates the harness after the editor's confirmed write.
func (a qualityEditorApply) run(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, retryCommand string) error {
	return applyQualityHarnessWith(cmd, dir, cfg, retryCommand, a.snapshot)
}

// runQualityTargetMenu is the guided flow `auto quality` ran before the
// composition step: the target menu when a tool carries per-agent models,
// otherwise the global quality menu.
func runQualityTargetMenu(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	apply qualityEditorApply,
	deps ompPlatformDependencies,
) error {
	if len(configuredQualityModelTools(cfg)) == 0 {
		return runQualityGlobalMenu(cmd, dir, cfg, apply)
	}
	target, err := chooseQualityTarget(cmd, cfg)
	if err != nil {
		return err
	}
	return runQualityTarget(cmd, dir, cfg, target, apply, deps)
}

// runQualityTarget runs the editor of one target chooseQualityTarget offers;
// global and any name that is not an installed tool open the global menu.
func runQualityTarget(
	cmd *cobra.Command,
	dir string,
	cfg *config.HarnessConfig,
	target string,
	apply qualityEditorApply,
	deps ompPlatformDependencies,
) error {
	for _, tool := range configuredQualityModelTools(cfg) {
		if tool.platform != target {
			continue
		}
		if tool.platform == "omp" {
			return runOMPQualityInteractive(cmd, dir, cfg, deps)
		}
		return runQualityToolInteractive(cmd, dir, cfg, tool, apply)
	}
	return runQualityGlobalMenu(cmd, dir, cfg, apply)
}

// runQualityGlobalMenu chooses quality.default among the configured presets.
func runQualityGlobalMenu(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, apply qualityEditorApply) error {
	options := orderedQualityPresets(cfg)
	if len(options) == 0 {
		return fmt.Errorf("quality presets are not configured")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Current quality: %s\n", cfg.Quality.Default)
	fmt.Fprintln(out, "Select quality mode:")
	for i, preset := range options {
		marker := " "
		if preset == cfg.Quality.Default {
			marker = "*"
		}
		// A description is free text from autopus.yaml (REQ-MUX-05).
		desc := terminalSafe(strings.TrimSpace(cfg.Quality.Presets[preset].Description))
		if desc != "" {
			fmt.Fprintf(out, "  %s %d) %s - %s\n", marker, i+1, preset, desc)
		} else {
			fmt.Fprintf(out, "  %s %d) %s\n", marker, i+1, preset)
		}
	}
	fmt.Fprint(out, "Choose: ")

	choice, err := readQualityChoice(cmd, options)
	if err != nil {
		return err
	}
	if err := saveQualityDefault(dir, cfg, choice); err != nil {
		return err
	}
	fmt.Fprintf(out, "quality.default = %s\n", choice)
	if apply.enabled {
		return apply.run(cmd, dir, cfg, "auto quality "+choice+" --apply")
	}
	return nil
}

// writeInheritOnlyQualityToolNote names the installed platforms whose agents
// carry no model, the note the target menu prints under its choices.
func writeInheritOnlyQualityToolNote(out io.Writer, cfg *config.HarnessConfig) {
	if inherited := inheritOnlyQualityToolLabels(cfg); len(inherited) > 0 {
		fmt.Fprintf(out, "%s inherit the session model and have no per-agent model to set.\n",
			strings.Join(inherited, " and "))
	}
}
