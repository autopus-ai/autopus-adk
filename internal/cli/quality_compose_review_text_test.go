package cli

// SPEC-REVMODEL-001 T10 (REQ-RVM-17): every review tab string ships in both
// languages with the same format verbs, and only ko reads Korean.

import (
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// reviewFormatVerbs lists a string's verbs with their argument indexes,
// sorted: the locales may order the arguments differently but must use the
// same ones.
func reviewFormatVerbs(s string) []string {
	verbs := regexp.MustCompile(`%(\[\d+\])?[a-z]`).FindAllString(s, -1)
	slices.Sort(verbs)
	return verbs
}

// reviewSameInBothLocales are the strings that name keys, selectors, or
// columns the way autopus.yaml and the runtime spell them.
var reviewSameInBothLocales = map[string]bool{
	"Model": true, "Thinking": true, "Judge": true, "ColProvider": true, "ColModel": true,
	"CLIBackend": true, "SummaryJudge": true, "Effort": true, "ColBackend": true,
}

func TestComposeReviewText_EveryStringIsTranslated(t *testing.T) {
	ko := reflect.ValueOf(composeReviewTextFor("ko"))
	en := reflect.ValueOf(composeReviewTextFor("en"))
	for i := range ko.NumField() {
		name := ko.Type().Field(i).Name
		koText, enText := ko.Field(i).String(), en.Field(i).String()
		assert.NotEmpty(t, koText, name)
		assert.NotEmpty(t, enText, name)
		assert.Equal(t, reviewFormatVerbs(koText), reviewFormatVerbs(enText), name)
		if !reviewSameInBothLocales[name] {
			assert.NotEqual(t, koText, enText, name)
		}
	}
	assert.Equal(t, []string{"%[1]s", "%[2]s", "%[3]s"}, reviewFormatVerbs(composeReviewTextEN.ErrThinking))
	for _, lang := range []string{"en", "ja", ""} {
		assert.Equal(t, composeReviewTextEN, composeReviewTextFor(lang), "lang %q", lang)
	}
}
