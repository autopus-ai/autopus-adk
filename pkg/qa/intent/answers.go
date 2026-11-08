package intent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Who resolved a question. A recommended answer is the reading the criterion
// was written with, applied without asking anyone; it is recorded as such so
// a reviewer can tell a choice from a default.
const (
	AnsweredByUser        = "user"
	AnsweredByRecommended = "recommended"
)

// CodeAnswerInvalid rejects an answer that names no question or option.
const CodeAnswerInvalid = "qa_intent_answer_invalid"

// QuestionsPath is where an intent keeps its questions and their answers.
func QuestionsPath(projectDir, id string) string {
	return filepath.Join(Dir(projectDir, id), "questions.json")
}

// LoadQuestions reads an intent's questions. A SPEC written by hand has no
// questions.json, which is not an error.
func LoadQuestions(projectDir, id string) ([]Question, error) {
	data, err := os.ReadFile(QuestionsPath(projectDir, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var questions []Question
	if err := json.Unmarshal(data, &questions); err != nil {
		return nil, fmt.Errorf("%s: %w", QuestionsPath(projectDir, id), err)
	}
	return questions, nil
}

// Open lists the questions nobody has answered.
func Open(questions []Question) []Question {
	out := []Question{}
	for _, q := range questions {
		if q.Answer == nil {
			out = append(out, q)
		}
	}
	return out
}

// Answer records picks, keyed by question id and indexing Options from 0.
// Every pick is checked before anything is written.
func Answer(projectDir, id string, picks map[string]int, by string) ([]Question, error) {
	questions, err := LoadQuestions(projectDir, id)
	if err != nil {
		return nil, err
	}
	if questions == nil {
		return nil, &Error{Code: CodeAnswerInvalid, Message: QuestionsPath(projectDir, id) + " does not exist"}
	}
	index := map[string]int{}
	for i, q := range questions {
		index[q.ID] = i
	}
	ids := make([]string, 0, len(picks))
	for qid := range picks {
		ids = append(ids, qid)
	}
	sort.Strings(ids)
	for _, qid := range ids {
		i, ok := index[qid]
		if !ok {
			return nil, &Error{Code: CodeAnswerInvalid, Message: fmt.Sprintf("%s has no question %s", id, qid)}
		}
		pick := picks[qid]
		if pick < 0 || pick >= len(questions[i].Options) {
			return nil, &Error{Code: CodeAnswerInvalid, Message: fmt.Sprintf(
				"%s has options 1-%d; got %d", qid, len(questions[i].Options), pick+1)}
		}
		questions[i].Answer = &pick
		questions[i].AnsweredBy = by
	}
	return questions, saveQuestions(projectDir, id, questions)
}

// AnswerRecommended resolves every open question with its recommended option.
func AnswerRecommended(projectDir, id string) ([]Question, error) {
	questions, err := LoadQuestions(projectDir, id)
	if err != nil {
		return nil, err
	}
	if questions == nil {
		return nil, &Error{Code: CodeAnswerInvalid, Message: QuestionsPath(projectDir, id) + " does not exist"}
	}
	picks := map[string]int{}
	for _, q := range Open(questions) {
		picks[q.ID] = q.Recommended
	}
	if len(picks) == 0 {
		return questions, nil
	}
	return Answer(projectDir, id, picks, AnsweredByRecommended)
}

// Readings renders answered questions for a prompt: one line each, naming the
// criterion, the question, and the chosen option.
func Readings(questions []Question) []string {
	out := []string{}
	for _, q := range questions {
		if q.Answer == nil || *q.Answer < 0 || *q.Answer >= len(q.Options) {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s -> %s (%s)", q.Criterion, oneLine(q.Question), oneLine(q.Options[*q.Answer]), q.AnsweredBy))
	}
	return out
}

func saveQuestions(projectDir, id string, questions []Question) error {
	data, err := json.MarshalIndent(questions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(QuestionsPath(projectDir, id), append(data, '\n'), 0o644)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Undecided renders open questions for a prompt, so a generator asserts
// neither reading instead of guessing one.
func Undecided(questions []Question) []string {
	out := []string{}
	for _, q := range Open(questions) {
		out = append(out, fmt.Sprintf("%s: %s (undecided between: %s)", q.Criterion, oneLine(q.Question), strings.Join(q.Options, " | ")))
	}
	return out
}
