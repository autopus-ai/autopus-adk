package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

type qaIntentAnswerOptions struct {
	ProjectDir  string
	Recommended bool
	JSONOut     bool
	Format      string
}

// qaIntentAnswerPayload is the JSON data of an answer run.
type qaIntentAnswerPayload struct {
	ID        string            `json:"id"`
	Questions []intent.Question `json:"questions"`
	Open      []string          `json:"open"`
}

func newQAIntentAnswerCmd() *cobra.Command {
	var opts qaIntentAnswerOptions
	cmd := &cobra.Command{
		Use:   "answer <ID> [Q1=<n> ...]",
		Short: "Record the user's answers to an intent's questions",
		Long: "Records which option the user chose for each question in questions.json. Options are numbered " +
			"from 1, as `auto qa intent` prints them. --recommended answers every question still open with its " +
			"recommended option and records that nobody chose it. Scenario generation reads the answers; " +
			"`auto qa go` will not generate while a question is open unless --auto is given.",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQAIntentAnswer(cmd, args[0], args[1:], opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Recommended, "recommended", false, "Answer every open question with its recommended option")
	cmd.Flags().StringVar(&opts.ProjectDir, "project-dir", ".", "Project directory")
	addJSONFlags(cmd, &opts.JSONOut, &opts.Format)
	return cmd
}

func runQAIntentAnswer(cmd *cobra.Command, id string, pairs []string, opts qaIntentAnswerOptions) error {
	jsonMode, err := resolveJSONMode(opts.JSONOut, opts.Format)
	if err != nil {
		return err
	}
	fail := func(cause error) error {
		if jsonMode {
			return writeJSONResultAndExit(cmd, jsonStatusError, cause, intent.ErrorCode(cause), nil, nil, nil)
		}
		return cause
	}
	picks, err := parseQAIntentPicks(pairs)
	if err != nil {
		return fail(err)
	}
	if len(picks) == 0 && !opts.Recommended {
		return fail(&intent.Error{Code: intent.CodeAnswerInvalid, Message: "give answers such as Q1=2, or --recommended"})
	}
	var questions []intent.Question
	if len(picks) > 0 {
		if questions, err = intent.Answer(opts.ProjectDir, id, picks, intent.AnsweredByUser); err != nil {
			return fail(err)
		}
	}
	if opts.Recommended {
		if questions, err = intent.AnswerRecommended(opts.ProjectDir, id); err != nil {
			return fail(err)
		}
	}
	payload := qaIntentAnswerPayload{ID: id, Questions: questions, Open: []string{}}
	for _, q := range intent.Open(questions) {
		payload.Open = append(payload.Open, q.ID)
	}
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, nil)
	}
	writeQAIntentAnswerText(cmd.OutOrStdout(), payload, opts.ProjectDir)
	return nil
}

// parseQAIntentPicks reads Q1=2 pairs into zero-based option indexes.
func parseQAIntentPicks(pairs []string) (map[string]int, error) {
	picks := map[string]int{}
	for _, pair := range pairs {
		qid, value, ok := strings.Cut(pair, "=")
		n, err := strconv.Atoi(value)
		if !ok || strings.TrimSpace(qid) == "" || err != nil {
			return nil, &intent.Error{Code: intent.CodeAnswerInvalid, Message: fmt.Sprintf("%q is not Q<id>=<option number>", pair)}
		}
		picks[strings.TrimSpace(qid)] = n - 1
	}
	return picks, nil
}

func writeQAIntentAnswerText(out io.Writer, payload qaIntentAnswerPayload, projectDir string) {
	for _, q := range payload.Questions {
		answer := "open"
		if q.Answer != nil {
			answer = fmt.Sprintf("%d. %s (%s)", *q.Answer+1, q.Options[*q.Answer], q.AnsweredBy)
		}
		fmt.Fprintf(out, "  %s (%s) %s\n", q.ID, q.Criterion, answer)
	}
	if len(payload.Open) > 0 {
		fmt.Fprintf(out, "still open: %s\n", strings.Join(payload.Open, ", "))
		return
	}
	fmt.Fprintf(out, "next: auto qa go %s --project-dir %s\n", payload.ID, projectDir)
}
