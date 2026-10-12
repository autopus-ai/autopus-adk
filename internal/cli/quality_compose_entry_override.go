package cli

import (
	"bufio"
	"fmt"
	"maps"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeOverrideText is the provider override question in one locale
// (REQ-MUX-22). Nothing written to autopus.yaml depends on it.
type composeOverrideText struct {
	question string // platform label, provider, override preset
	removed  string // provider
}

var composeOverrideTexts = map[string]composeOverrideText{
	"ko": {
		question: "%s도 새 구성을 따를까요? (quality.providers.%s = %s 제거) [y/N]",
		removed:  "quality.providers.%s 제거",
	},
	"en": {
		question: "Should %s follow the new composition too? (removes quality.providers.%s = %s) [y/N]",
		removed:  "quality.providers.%s removed",
	},
}

func composeOverrideTextFor(lang string) composeOverrideText {
	if lang == "ko" {
		return composeOverrideTexts["ko"]
	}
	return composeOverrideTexts["en"]
}

// composeOverrideProviders lists the provider override columns in table
// order with the platform label the question names.
var composeOverrideProviders = []struct{ provider, label string }{
	{config.QualityProviderClaude, "Claude Code"},
	{config.QualityProviderCodex, "Codex"},
}

// composeOverridePlanner is the session planner with the provider override
// question (SPEC-MODELUX-001 REQ-MUX-20): after an edit that writes and
// before its preview, it asks whether each overridden provider follows the
// new composition. The answer goes into the candidate first, so the
// after-table, the change lines, and the generation all show it, and a
// confirmed removal goes through the existing provider writer once the
// composition is written.
type composeOverridePlanner struct {
	*composeSessionPlanner
	cmd *cobra.Command
	// form asks through a huh Confirm whose default is no and joins a
	// confirmed removal to the composition write, one quality block edit
	// that leaves both or neither (SPEC-MODELUX-002 REQ-MUX2-18). The line
	// driver asks a line and removes afterwards through the provider writer.
	form bool
}

// withComposeOverrideQuestion adds the override question to a session built
// by newComposeSession. The question reads the session's shared input
// reader, which the driver installs on cmd.
func withComposeOverrideQuestion(cmd *cobra.Command, session composeDriverSession) composeDriverSession {
	if planner, ok := session.Planner.(*composeSessionPlanner); ok {
		session.Planner = composeOverridePlanner{composeSessionPlanner: planner, cmd: cmd}
	}
	return session
}

// PlanEdit plans edited exactly as composeSessionPlanner.PlanEdit does, with
// the override question between the binding plan and its description, so
// the Codex default session line is resolved without a removed override.
func (p composeOverridePlanner) PlanEdit(edited map[string]string) (composeDriverPlan, error) {
	binding, composition := p.bind(edited)
	if binding.Writer == composeWriterNone {
		return composeDriverPlan{NoChange: true}, nil
	}
	followers, err := p.askOverrides(binding.Candidate)
	if err != nil {
		return composeDriverPlan{}, err
	}
	for _, provider := range followers {
		delete(binding.Candidate.Quality.Providers, provider)
	}
	if p.form && len(followers) > 0 {
		binding.Writer = composeWriterBlock
	}
	plan, err := p.driverPlan(binding, composition)
	if err != nil || len(followers) == 0 {
		return plan, err
	}
	text := composeOverrideTextFor(p.run.lang)
	for _, provider := range followers {
		plan.Changes = append(plan.Changes, fmt.Sprintf(text.removed, provider))
	}
	if p.form {
		plan.Apply = func() error { return p.persistJoined(binding, followers) }
		return plan, nil
	}
	write := plan.Apply
	plan.Apply = func() error {
		if err := write(); err != nil {
			return err
		}
		return p.removeOverrides(followers)
	}
	return plan, nil
}

// askOverrides asks about every installed provider whose override is in
// force and names another preset than the new global binding. On the line
// driver only y or yes follows; Enter, input end, and any other answer keep
// the override. A form defaults to no the same way; Ctrl+C ends the step.
func (p composeOverridePlanner) askOverrides(candidate *config.HarnessConfig) ([]string, error) {
	text := composeOverrideTextFor(p.run.lang)
	out := p.cmd.OutOrStdout()
	reader := bufio.NewReader(p.cmd.InOrStdin())
	binding := candidate.Quality.EffectiveMode("")
	var followers []string
	for _, column := range composeOverrideProviders {
		override := composeTableProviderOverride(candidate.Quality, column.provider)
		if override == "" || override == binding ||
			!containsPlatform(candidate.Platforms, qualityProviderPlatform(column.provider)) {
			continue
		}
		question := fmt.Sprintf(text.question, column.label, column.provider, terminalSafe(override))
		if p.form {
			follow, err := composeConfirm(p.cmd, strings.TrimSuffix(question, " [y/N]"))
			if err != nil {
				return nil, err
			}
			if follow {
				followers = append(followers, column.provider)
			}
			continue
		}
		fmt.Fprintln(out, question)
		fmt.Fprint(out, "> ")
		line, err := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if err != nil && answer == "" {
			fmt.Fprintln(out)
		}
		if answer == "y" || answer == "yes" {
			followers = append(followers, column.provider)
		}
	}
	return followers, nil
}

// persistJoined writes the composition and the confirmed removals in one
// edit: one saveQualityBlock splice and atomic replace for a file, which
// leaves the file and cfg as they were on refusal or failure, and for init
// one change of its config after the plan's built-in checks.
func (p composeOverridePlanner) persistJoined(binding composeBindingPlan, followers []string) error {
	edit := func(quality *config.QualityConf) error {
		if err := binding.apply(quality); err != nil {
			return err
		}
		for _, provider := range followers {
			delete(quality.Providers, provider)
		}
		return nil
	}
	if p.run.writer == composeSessionMemory {
		return edit(&p.run.cfg.Quality)
	}
	return saveQualityBlock(p.run.dir, p.run.cfg, edit)
}

// removeOverrides drops each confirmed override after the composition write,
// one quality.providers line at a time through the provider writer. The
// config follows each removal only once it is written; init's memory writer
// has no file, so its config drops them directly.
func (p composeOverridePlanner) removeOverrides(providers []string) error {
	cfg := p.run.cfg
	for _, provider := range providers {
		if p.run.writer == composeSessionMemory {
			delete(cfg.Quality.Providers, provider)
			continue
		}
		staged := *cfg
		staged.Quality.Providers = maps.Clone(cfg.Quality.Providers)
		if err := removeQualityProvider(p.run.dir, &staged, provider); err != nil {
			return fmt.Errorf("quality.providers.%s was not removed: %w", provider, err)
		}
		cfg.Quality.Providers = staged.Quality.Providers
	}
	return nil
}
