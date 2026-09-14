package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompCustomBaseProfile is the profile a per-agent selection layers onto. A pin
// governs the agents the operator names; every other bundled agent still needs
// a route, and the balanced built-in is the one the harness can derive.
const ompCustomBaseProfile = "balanced"

// ompCustomModelChoice is one selectable installed model. Provider and model
// are kept apart because a pin is written as provider/model[:thinking].
// Capabilities is empty on a metadata-light catalog, which is the difference
// between "this model cannot serve the agent" and "the installation does not
// say".
type ompCustomModelChoice struct {
	Provider     string
	Model        string
	Thinking     []string
	Capabilities []string
}

func (c ompCustomModelChoice) selector() string { return c.Provider + "/" + c.Model }

// readOMPCustomAgentPins asks for one installed model per bundled agent and
// returns them in `--agent` form, so the selection travels through the same
// attestation, preview, and rollback path as the flag.
func readOMPCustomAgentPins(
	ctx context.Context,
	cmd *cobra.Command,
	runner omp.OMPModelCatalogRunner,
) ([]string, error) {
	out := cmd.OutOrStdout()
	choices, version, err := installedOMPCustomModelChoices(ctx, cmd, runner)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "\nInstalled models (%s)\n", version)
	renderOMPCustomModelChoices(out, choices)
	fmt.Fprintf(out, "Pick a model per agent; empty keeps the %s default.\n", ompCustomBaseProfile)

	reader := bufio.NewReader(cmd.InOrStdin())
	pins := make([]string, 0, len(config.OMPNativeAgentNames()))
	for _, agent := range config.OMPNativeAgentNames() {
		capability, err := ompCustomAgentCapability(agent)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(out, "%s (%s) [1-%d, enter=default]: ", agent, capability, len(choices))
		choice, selected, err := readOMPCustomModelChoice(reader, choices)
		if err != nil {
			return nil, err
		}
		if err := requireOMPCustomCapability(choice, selected, capability, choices); err != nil {
			return nil, err
		}
		if !selected {
			continue
		}
		thinking, err := readOMPCustomThinkingChoice(cmd, reader, choice)
		if err != nil {
			return nil, err
		}
		pin := agent + "=" + choice.selector()
		if thinking != "" {
			pin += ":" + thinking
		}
		pins = append(pins, pin)
		fmt.Fprintf(out, "  %s\n", pin)
	}
	return pins, nil
}

// ompCustomAgentCapability names the provider-neutral capability the bundled
// agent's route carries, so the prompt states what the model has to serve
// instead of letting the operator discover it from a later blocker.
func ompCustomAgentCapability(agent string) (string, error) {
	resolved, err := config.ResolveOMPPolicyAgent(agent)
	if err != nil {
		return "", err
	}
	return resolved.Capability, nil
}

// requireOMPCustomCapability rejects a pin the catalog says cannot serve the
// agent, naming the models that can. The plan gate would refuse it anyway,
// after the operator answered every remaining prompt.
func requireOMPCustomCapability(
	choice ompCustomModelChoice,
	selected bool,
	capability string,
	choices []ompCustomModelChoice,
) error {
	if !selected || len(choice.Capabilities) == 0 ||
		containsOMPString(choice.Capabilities, capability) {
		return nil
	}
	eligible := make([]string, 0, len(choices))
	for _, candidate := range choices {
		if containsOMPString(candidate.Capabilities, capability) {
			eligible = append(eligible, candidate.selector())
		}
	}
	if len(eligible) == 0 {
		return fmt.Errorf(
			"no installed model declares %s, which this agent's route requires", capability)
	}
	return fmt.Errorf(
		"model %s does not declare %s (models that do: %s)",
		choice.selector(), capability, strings.Join(eligible, ", "),
	)
}

// installedOMPCustomModelChoices lists what the installation actually ships.
// A strict catalog carries family and capability metadata; a metadata-light one
// still reports exact selectors and thinking levels, and a pin made from those
// is attested against the built-in declarations exactly as `--agent` is. Any
// other probe failure is returned so the operator never picks from a guess.
func installedOMPCustomModelChoices(
	ctx context.Context,
	cmd *cobra.Command,
	runner omp.OMPModelCatalogRunner,
) ([]ompCustomModelChoice, string, error) {
	probe, err := probeInstalledOMPCatalog(ctx, runner)
	version := safeOMPOperatorVersion(probe.Version)
	switch {
	case err == nil:
		return sortedOMPCustomModelChoices(strictOMPCustomModelChoices(probe.Catalog.Models)), version, nil
	case probe.Reason == ompRawCatalogFallbackReason:
		body, runErr := runner.Run(ctx, "omp", "models", "--json", "--no-extensions")
		if runErr != nil {
			return nil, version, runErr
		}
		models, reason := normalizeOMPRawDisplayCatalog(body, ompModelDoctorProbeOutput)
		if reason != "catalog_ready" {
			return nil, version, ompCatalogUnavailableError{reason: reason}
		}
		fmt.Fprintln(cmd.OutOrStdout(),
			"The installed catalog reports no family or capability metadata; "+
				"a pin is attested against the built-in declarations.")
		return sortedOMPCustomModelChoices(displayOMPCustomModelChoices(models)), version, nil
	default:
		return nil, version, err
	}
}

// strictOMPCustomModelChoices keeps only models the installation can run and
// that declare a thinking level, because a pin carries one.
func strictOMPCustomModelChoices(models []omp.OMPModelMetadata) []ompCustomModelChoice {
	choices := make([]ompCustomModelChoice, 0, len(models))
	for _, model := range models {
		if ompModelAvailability(model) != ompProfileAvailabilityAvailable || len(model.Thinking) == 0 {
			continue
		}
		choices = append(choices, ompCustomModelChoice{
			Provider: model.Provider, Model: model.Model,
			Thinking: model.Thinking, Capabilities: model.Capabilities,
		})
	}
	return choices
}

func displayOMPCustomModelChoices(models []ompCatalogModelPayload) []ompCustomModelChoice {
	choices := make([]ompCustomModelChoice, 0, len(models))
	for _, model := range models {
		thinking := model.Thinking
		if len(thinking) == 0 {
			thinking = model.NativeThinking
		}
		if model.Disabled || len(thinking) == 0 {
			continue
		}
		choices = append(choices, ompCustomModelChoice{
			Provider: model.Provider, Model: model.Model,
			Thinking: thinking, Capabilities: model.Capabilities,
		})
	}
	return choices
}

func sortedOMPCustomModelChoices(choices []ompCustomModelChoice) []ompCustomModelChoice {
	sort.Slice(choices, func(i, j int) bool {
		return choices[i].selector() < choices[j].selector()
	})
	return choices
}

func renderOMPCustomModelChoices(out io.Writer, choices []ompCustomModelChoice) {
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "#\tModel\tThinking")
	for index, choice := range choices {
		fmt.Fprintf(table, "%d\t%s\t%s\n",
			index+1, choice.selector(), strings.Join(choice.Thinking, ","))
	}
	_ = table.Flush()
}

func readOMPCustomModelChoice(
	reader *bufio.Reader,
	choices []ompCustomModelChoice,
) (ompCustomModelChoice, bool, error) {
	if len(choices) == 0 {
		return ompCustomModelChoice{}, false, errors.New("the installed OMP catalog reports no usable model")
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return ompCustomModelChoice{}, false, fmt.Errorf("read model choice: %w", err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return ompCustomModelChoice{}, false, nil
	}
	index, convErr := strconv.Atoi(answer)
	if convErr != nil || index < 1 || index > len(choices) {
		return ompCustomModelChoice{}, false, fmt.Errorf(
			"invalid model choice %q (expected 1-%d or empty)", answer, len(choices))
	}
	return choices[index-1], true, nil
}

// readOMPCustomThinkingChoice asks for a thinking level only when the model
// offers more than one. A single-level model has nothing to choose, and asking
// would invite a level the installation does not support.
func readOMPCustomThinkingChoice(
	cmd *cobra.Command,
	reader *bufio.Reader,
	choice ompCustomModelChoice,
) (string, error) {
	if len(choice.Thinking) == 1 {
		return choice.Thinking[0], nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  thinking [%s, enter=%s]: ",
		strings.Join(choice.Thinking, "/"), choice.Thinking[0])
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read thinking choice: %w", err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return choice.Thinking[0], nil
	}
	for _, level := range choice.Thinking {
		if strings.EqualFold(level, answer) {
			return level, nil
		}
	}
	return "", fmt.Errorf(
		"model %s does not support thinking %q (supported: %s)",
		choice.selector(), answer, strings.Join(choice.Thinking, ", "),
	)
}
