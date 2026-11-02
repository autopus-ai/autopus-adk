package worker

import (
	"regexp"
	"sort"
	"strings"
)

// Fallback mode (SPEC-WORKERPOST-001 REQ-WPOST-LEGACY-01..03) serves payloads
// that carry no completion contract. It keeps the exact words the pre-contract
// inference matched (`commit`, `push`, `branch`) and only drops the ones the
// prompt negates, so it can require less than before, never more.

var (
	refsHeadsPattern     = regexp.MustCompile(`refs/heads/([A-Za-z0-9._/-]+)`)
	branchKeywordPattern = regexp.MustCompile(`(?i)\bbranch(?:\s+(?:named|called))?\s+([A-Za-z0-9._/-]*[/-][A-Za-z0-9._/-]+)\b`)
	originBranchPattern  = regexp.MustCompile(`(?i)\borigin(?:/|\s+)([A-Za-z0-9._/-]*[/-][A-Za-z0-9._/-]+)\b`)
	promptClausePattern  = regexp.MustCompile(`[.;!?\r\n]+`)
	promptTokenPattern   = regexp.MustCompile(`[a-z0-9_]+(?:'[a-z]+)?|\p{Hangul}+|[,/]`)
)

// actionWords maps every inflection the negation grammar recognizes to its
// action. Only the bare word counts as an affirmative request.
var actionWords = map[string]string{
	"commit": actionCommit, "commits": actionCommit, "committed": actionCommit, "committing": actionCommit,
	"push": actionPush, "pushes": actionPush, "pushed": actionPush, "pushing": actionPush,
	"branch": actionBranch, "branches": actionBranch,
}

var (
	negationCues = wordSet("not", "no", "never", "without", "nor", "don't", "dont", "doesn't", "didn't",
		"mustn't", "shouldn't", "won't", "can't", "cannot", "avoid", "avoiding", "refrain", "skip",
		"forbid", "prohibit")
	negationFillers = wordSet("a", "an", "the", "any", "your", "our", "this", "these", "those", "it", "them",
		"change", "changes", "work", "git", "to", "be", "being", "been", "yet", "ever", "anything", "new",
		"create", "creating", "make", "making", "perform", "performing", "run", "running", "do", "doing",
		"try", "attempt", "need", "force", "from", "or", "and", ",", "/")
	prohibitionWords  = wordSet("forbidden", "prohibited", "disallowed", "banned")
	notPermittedWords = wordSet("allowed", "permitted", "needed", "required", "necessary")
)

// postPositiveWindow is how many tokens after an action word a prohibition
// such as "is not allowed" may start.
const postPositiveWindow = 4

type actionMentions struct {
	affirmative bool
	negated     bool
}

// detectTaskPostconditions infers the fallback plan from the prompt.
func detectTaskPostconditions(prompt string) taskPostconditions {
	mentions := scanActionMentions(prompt)
	plan := taskPostconditions{
		Source:    sourceLegacy,
		Commit:    fallbackLevel(mentions[actionCommit]),
		Push:      fallbackLevel(mentions[actionPush]),
		Branch:    levelNone,
		Remote:    defaultPostconditionRemote,
		Match:     matchExact,
		Branches:  extractPromptBranches(prompt),
		Mentioned: len(mentions) > 0,
	}
	// A branch requirement without a name was vacuous before: any attached
	// HEAD satisfied it outside a worktree and nothing could inside one.
	if len(plan.Branches) > 0 {
		plan.Branch = fallbackLevel(mentions[actionBranch])
	}
	return plan
}

func fallbackLevel(m actionMentions) requirementLevel {
	switch {
	case m.affirmative && m.negated:
		return levelAmbiguous
	case m.affirmative:
		return levelRequired
	default:
		return levelNone
	}
}

// scanActionMentions classifies every action word of the prompt, clause by
// clause, as an affirmative request or a negated mention.
func scanActionMentions(prompt string) map[string]actionMentions {
	text := strings.ToLower(strings.ReplaceAll(prompt, "’", "'"))
	found := make(map[string]actionMentions)
	for _, clause := range promptClausePattern.Split(text, -1) {
		tokens := promptTokenPattern.FindAllString(clause, -1)
		for i, token := range tokens {
			tokens[i] = strings.TrimSuffix(token, "'s")
		}
		negated := negatedTokens(tokens)
		for i, token := range tokens {
			action, ok := actionWords[token]
			if !ok {
				continue
			}
			mention := found[action]
			switch {
			case negated[i]:
				mention.negated = true
			case token == action:
				mention.affirmative = true
			default:
				// Affirmative inflections ("pushed") were never requests.
				continue
			}
			found[action] = mention
		}
	}
	return found
}

// negatedTokens marks the action words a negation governs: a cue followed by
// fillers and coordinated action words ("do not commit or push"), or a
// prohibition that follows the action ("pushing to main is not allowed").
func negatedTokens(tokens []string) []bool {
	negated := make([]bool, len(tokens))
	for i, token := range tokens {
		if !negationCues[token] {
			continue
		}
		for j := i + 1; j < len(tokens); j++ {
			next := tokens[j]
			if _, ok := actionWords[next]; ok {
				negated[j] = true
				continue
			}
			if !negationFillers[next] && !negationCues[next] {
				break
			}
		}
	}
	for i, token := range tokens {
		if _, ok := actionWords[token]; ok && prohibitionFollows(tokens, i) {
			negated[i] = true
		}
	}
	return negated
}

func prohibitionFollows(tokens []string, at int) bool {
	for j := at + 1; j < len(tokens) && j <= at+postPositiveWindow; j++ {
		token := tokens[j]
		if _, ok := actionWords[token]; ok || token == "," {
			return false
		}
		if prohibitionWords[token] || strings.HasPrefix(token, "금지") || koreanProhibition(token) {
			return true
		}
		if j+1 < len(tokens) {
			next := tokens[j+1]
			if token == "not" && notPermittedWords[next] {
				return true
			}
			if token == "하지" && hasAnyPrefix(next, "마", "말", "않") {
				return true
			}
		}
	}
	return false
}

// koreanProhibition matches the run-together forms 하지마, 하지말, 하지않.
func koreanProhibition(token string) bool {
	return hasAnyPrefix(token, "하지마", "하지말", "하지않")
}

func hasAnyPrefix(token string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}

// extractPromptBranches returns the branch names the pre-contract patterns
// find, sorted and de-duplicated.
func extractPromptBranches(prompt string) []string {
	seen := make(map[string]struct{})
	for _, pattern := range []*regexp.Regexp{refsHeadsPattern, branchKeywordPattern, originBranchPattern} {
		for _, match := range pattern.FindAllStringSubmatch(prompt, -1) {
			if len(match) < 2 {
				continue
			}
			if branch := sanitizeBranchName(match[1]); branch != "" {
				seen[branch] = struct{}{}
			}
		}
	}
	branches := make([]string, 0, len(seen))
	for branch := range seen {
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches
}

// fallbackPushRef picks the ref a fallback push is judged on
// (REQ-WPOST-LEGACY-02); "" means the prompt leaves the target unclear.
func fallbackPushRef(plan taskPostconditions, currentBranch string) string {
	current := sanitizeBranchName(currentBranch)
	usable := current != "" && !strings.HasPrefix(current, "worker-")
	if usable && (len(plan.Branches) == 0 || containsBranch(plan.Branches, current)) {
		return branchRefPrefix + current
	}
	if len(plan.Branches) == 1 {
		return branchRefPrefix + plan.Branches[0]
	}
	return ""
}

func containsBranch(branches []string, branch string) bool {
	for _, candidate := range branches {
		if candidate == branch {
			return true
		}
	}
	return false
}

func sanitizeBranchName(raw string) string {
	branch := strings.TrimSpace(raw)
	branch = strings.Trim(branch, "\"'`.,)")
	if branch == "" {
		return ""
	}
	if strings.Contains(branch, "..") || strings.HasPrefix(branch, "-") {
		return ""
	}
	return branch
}

func wordSet(words ...string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}
