package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// harnessFeedbackEnv turns the loop off: "off" stops the Stop hook from
// starting an observation. The observation sets it for the providers it
// runs, so their own sessions do not feed the loop.
const harnessFeedbackEnv = "AUTOPUS_HARNESS_FEEDBACK"

// harnessWindow is how far back the report and the automatic proposal look.
const harnessWindow = 30 * 24 * time.Hour

var (
	harnessNow       = time.Now
	harnessProviders = func() ([]string, error) { return checklistProviders() }
	harnessAsker     = defaultHarnessAsker
	harnessSpawn     = spawnHarnessObserve
)

func newHarnessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "harness",
		Short: "Measure where session time goes and turn repeated waste into rules a person approves",
		Long: "The harness feedback loop. The Claude Code Stop hook records each session's test waits, reviews, repeat reviews, " +
			"hook blocks and timeouts under .autopus/runtime/harness (no model call). When a waste pattern shows up in " +
			fmt.Sprintf("%d or more sessions, the same background run drafts a rule for it with one review provider and asks ", harnessfb.MinSessions) +
			"another to refute it. approve writes a surviving rule into " + harnessfb.RuleFile + "; that is a person's decision. " +
			"encode then proves a check in the repository that enforces the lesson, and the text rule leaves the file. " +
			harnessFeedbackEnv + "=off turns the loop off.",
	}
	cmd.AddCommand(newHarnessObserveCmd(), newHarnessReportCmd(), newHarnessProposeCmd(), newHarnessRefuteCmd(),
		newHarnessStatusCmd(), newHarnessDecideCmd("approve"), newHarnessDecideCmd("reject"), newHarnessDecideCmd("retire"), newHarnessEncodeCmd())
	return cmd
}

func harnessRoot(ctx context.Context) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return harnessfb.ProjectRoot(ctx, cwd), nil
}

// maybeStartHarnessObserve starts the observation of a stopping session in
// the background. The Stop hook's own result never depends on it.
func maybeStartHarnessObserve(in stopHookInput, dir string) {
	// A Codex stop (turn_id set) hands over a Codex rollout, not a Claude
	// Code transcript: recorded, it would be an empty session that only
	// grows the denominators.
	if strings.EqualFold(os.Getenv(harnessFeedbackEnv), "off") || in.SessionID == "" || in.TranscriptPath == "" || dir == "" ||
		in.TurnID != "" {
		return
	}
	_ = harnessSpawn(in.SessionID, in.TranscriptPath, dir)
}

func spawnHarnessObserve(session, transcript, dir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "harness", "observe", "--session-id", session, "--transcript", transcript)
	cmd.Dir = dir
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func newHarnessObserveCmd() *cobra.Command {
	var session, transcript string
	var noPropose bool
	cmd := &cobra.Command{
		Use:    "observe",
		Short:  "Record a session's time from its transcript (the Stop hook runs this in the background)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			root, err := harnessRoot(ctx)
			if err != nil {
				return err
			}
			if _, err := harnessfb.Observe(ctx, root, session, transcript); err != nil {
				if errors.Is(err, harnessfb.ErrBusy) {
					return nil
				}
				return err
			}
			// A worktree made after the last approval gets the rules too.
			if cwd, err := os.Getwd(); err == nil {
				_ = harnessfb.SyncRules(root, worktreeTop(ctx, cwd))
			}
			if noPropose {
				return nil
			}
			// The providers below run sessions of their own; they must not
			// feed this loop.
			_ = os.Setenv(harnessFeedbackEnv, "off")
			say := func(msg string) { _ = harnessfb.AppendLog(root, harnessNow(), msg) }
			return proposeAndRefute(ctx, root, say, true)
		},
	}
	cmd.Flags().StringVar(&session, "session-id", "", "Session id")
	cmd.Flags().StringVar(&transcript, "transcript", "", "Path of the session's Claude Code transcript")
	cmd.Flags().BoolVar(&noPropose, "no-propose", false, "Only record; do not draft candidates")
	return cmd
}

// worktreeTop is the top of the worktree dir is in.
func worktreeTop(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}

// proposeAndRefute drafts a candidate for each repeated pattern that has
// none and asks other providers to refute it. Approval stays a person's.
// The automatic run (auto) asks no model outside a git repository, where
// nothing it learns could be recorded, and waits out failed drafts.
func proposeAndRefute(ctx context.Context, root string, say func(string), auto bool) error {
	if _, err := harnessfb.GitDir(root); err != nil {
		if auto {
			return nil
		}
		return err
	}
	sessions, err := harnessfb.LoadSessions(root, harnessNow().Add(-harnessWindow))
	if err != nil {
		return err
	}
	cands, err := harnessfb.LoadCandidates(root)
	if err != nil {
		return err
	}
	patterns := harnessfb.Detect(sessions)
	if len(harnessfb.Unproposed(patterns, cands)) == 0 {
		return nil
	}
	providers, err := harnessProviders()
	if err != nil {
		return err
	}
	created, errs := harnessfb.Propose(ctx, root, patterns, providers, harnessAsker(root), harnessNow(), harnessfb.Options{Auto: auto})
	for _, e := range errs {
		say("후보 작성 실패: " + e.Error())
	}
	for _, c := range created {
		say(fmt.Sprintf("%s 후보 작성 (%s, 세션 %d개)", c.ID, c.Pattern, len(c.Evidence)))
		got, err := harnessfb.Refute(ctx, root, c, providers, harnessAsker(root), harnessNow())
		if err != nil {
			say(fmt.Sprintf("%s 반증 대기: %v", c.ID, err))
			continue
		}
		say(fmt.Sprintf("%s %s", got.ID, got.State.Label()))
	}
	if len(errs) > 0 {
		return fmt.Errorf("후보 %d개를 만들지 못했습니다", len(errs))
	}
	return nil
}

func defaultHarnessAsker(root string) harnessfb.Asker {
	return func(ctx context.Context, name, prompt string) (harnessfb.Reply, error) {
		conf, err := loadOrchestraConfig()
		if err != nil {
			return harnessfb.Reply{Tokens: -1}, err
		}
		providers, err := applyReadOnlyProviderPolicy(resolveProviders(conf, "review", []string{name}), readOnlyPolicyOptions{})
		if err != nil || len(providers) == 0 {
			return harnessfb.Reply{Tokens: -1}, fmt.Errorf("provider %s: %v", name, err)
		}
		p := providers[0]
		// The prompt carries everything the provider needs; in an empty
		// repository the CLI has no project files to browse with its read
		// tools (measured: a claude draft fell from ~51k to 11-19k tokens).
		dir, cleanup, err := harnessProviderDir(ctx)
		if err != nil {
			return harnessfb.Reply{Tokens: -1}, err
		}
		defer cleanup()
		cfg := orchestra.OrchestraConfig{
			Providers: []orchestra.ProviderConfig{p}, TimeoutSeconds: 600,
			WorkingDir: dir, ProviderWorkDir: dir, ReadOnly: true,
		}
		resp, err := orchestra.RunSingleProvider(ctx, cfg, p, prompt)
		if resp == nil {
			return harnessfb.Reply{Tokens: -1}, err
		}
		return harnessfb.Reply{Output: resp.Output, Seconds: resp.Duration.Seconds(), Tokens: usageTokens(resp)}, err
	}
}

// harnessDirPrefix names the empty repositories providers run in.
const harnessDirPrefix = "autopus-harness-"

// harnessProviderDir makes an empty git repository for one provider call
// and returns how to remove it. A run killed before its cleanup leaves one
// behind, so repositories older than an hour are removed first.
func harnessProviderDir(ctx context.Context) (string, func(), error) {
	tmp := os.TempDir()
	if entries, err := os.ReadDir(tmp); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), harnessDirPrefix) {
				continue
			}
			// An hour is well past a call's 600 s timeout (set below), so
			// no running call's directory is this old.
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Hour {
				_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
			}
		}
	}
	dir, err := os.MkdirTemp(tmp, harnessDirPrefix)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	// Some CLIs (codex exec) refuse to run outside a git repository.
	if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git init %s: %v: %s", dir, err, out)
	}
	return dir, cleanup, nil
}

// usageTokens sums the input and output tokens a provider reported, or -1
// when it reported none.
func usageTokens(resp *orchestra.ProviderResponse) int64 {
	var total int64 = -1
	for _, u := range resp.Usage {
		for _, n := range []*int64{u.InputTokensTotal, u.OutputTokensTotal} {
			if n != nil {
				total = max(total, 0) + *n
			}
		}
	}
	return total
}
