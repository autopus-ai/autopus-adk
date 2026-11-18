package checklist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/learn"
)

// Asker sends prompt to the named provider and returns its reply.
type Asker func(ctx context.Context, provider, prompt string) (string, error)

// MaxRefuters caps how many providers review one candidate.
const MaxRefuters = 2

// ErrNoRefuter means no provider other than the drafter is available.
var ErrNoRefuter = fmt.Errorf("다른 provider가 필요합니다: 초안을 쓴 provider 말고 사용할 수 있는 review provider가 없어 후보가 반증 검토 대기로 남습니다")

func describeExits(ev *learn.Evidence) string {
	exit := func(p *int) string {
		if p == nil {
			return "기록 없음"
		}
		return fmt.Sprint(*p)
	}
	return fmt.Sprintf("재현 명령 %q: 수정 전 exit %s, 수정 후 exit %s", ev.Command, exit(ev.BeforeExit), exit(ev.AfterExit))
}

// RefutePrompt asks a provider to try to refute the candidate.
func RefutePrompt(c Candidate, existing []Check) string {
	var b strings.Builder
	b.WriteString("You review a proposed harness check before a person decides on it. Try to refute it.\n")
	b.WriteString("Refute it when it duplicates an existing check below, when its condition cannot be identified from a diff, or when it fits only its source incident and would be noise elsewhere.\n\n")
	fmt.Fprintf(&b, "Proposed check %s\nCondition: paths %v, keywords %v\nVerify command: %q\nCriterion:\n%s\n", c.ID, c.Check.When.Paths, c.Check.When.Keywords, c.Check.Verify, c.Check.Body)
	fmt.Fprintf(&b, "Evidence: %d distinct occurrence(s)", c.Repeats)
	if c.Repro != nil {
		fmt.Fprintf(&b, "; repro %q failed before the fix (exit %d) and passed after it (exit %d)", c.Repro.Command, c.Repro.BeforeExit, c.Repro.AfterExit)
	}
	b.WriteString("\n\nExisting checks:\n")
	if len(existing) == 0 {
		b.WriteString("(none)\n")
	}
	for _, e := range existing {
		fmt.Fprintf(&b, "- %s paths %v keywords %v: %s\n", e.ID, e.When.Paths, e.When.Keywords, firstLine(e.Body))
	}
	b.WriteString("\nReply with exactly two lines and nothing else:\nVERDICT: refuted or survived\nREASON: one sentence\n")
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ParseVerdict reads the VERDICT and REASON lines of a reply.
func ParseVerdict(provider, reply string) Refutation {
	r := Refutation{Provider: provider, Verdict: "unparsed"}
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "*`"))
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "VERDICT:"):
			v := strings.ToLower(strings.TrimSpace(line[len("VERDICT:"):]))
			if v == "refuted" || v == "survived" {
				r.Verdict = v
			}
		case strings.HasPrefix(upper, "REASON:"):
			r.Reason = strings.TrimSpace(line[len("REASON:"):])
		}
	}
	if r.Verdict == "unparsed" && r.Reason == "" {
		r.Reason = firstLine(strings.TrimSpace(reply))
	}
	return r
}

// Refute asks up to MaxRefuters providers other than the drafter. Any
// refuted verdict refutes the candidate; otherwise one survived verdict
// sends it to the approval queue. With no usable verdict it stays pending.
func Refute(ctx context.Context, root string, c Candidate, providers []string, ask Asker, now time.Time) (Candidate, error) {
	if c.State != StatePendingRefutation {
		return c, fmt.Errorf("%s is %s, not %s", c.ID, c.State.Label(), StatePendingRefutation.Label())
	}
	var others []string
	for _, p := range providers {
		if p != c.DraftedBy && !contains(others, p) {
			others = append(others, p)
		}
	}
	if len(others) == 0 {
		return c, ErrNoRefuter
	}
	if len(others) > MaxRefuters {
		others = others[:MaxRefuters]
	}
	existing, _, err := WorkingChecks(root)
	if err != nil {
		return c, err
	}
	prompt := RefutePrompt(c, existing)
	survived, refuted := 0, 0
	for _, p := range others {
		reply, err := ask(ctx, p, prompt)
		r := ParseVerdict(p, reply)
		if err != nil {
			r = Refutation{Provider: p, Verdict: "error", Reason: err.Error()}
		}
		c.Refutations = append(c.Refutations, r)
		switch r.Verdict {
		case "refuted":
			refuted++
		case "survived":
			survived++
		}
	}
	switch {
	case refuted > 0:
		c.State = StateRefuted
		c.Decided = &now
	case survived > 0:
		c.State = StateAwaitingApproval
	}
	return c, SaveCandidate(root, c)
}
