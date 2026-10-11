package cli

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComposeTextFor_KoreanOnlyForKo pins REQ-MUX-22: the ko locale reads
// Korean and every other locale reads English.
func TestComposeTextFor_KoreanOnlyForKo(t *testing.T) {
	assert.Equal(t, "구성을 바꿀까요?", composeTextFor("ko").Question)
	for _, lang := range []string{"en", "ja", "zh", ""} {
		assert.Equal(t, "Change the composition?", composeTextFor(lang).Question, "lang %q", lang)
	}
}

// TestComposeText_EveryStringIsTranslated keeps a prompt from shipping in one
// language only, or with format verbs the other language lacks.
func TestComposeText_EveryStringIsTranslated(t *testing.T) {
	ko := reflect.ValueOf(composeTextFor("ko"))
	en := reflect.ValueOf(composeTextFor("en"))
	for i := range ko.NumField() {
		name := ko.Type().Field(i).Name
		koText, enText := ko.Field(i).String(), en.Field(i).String()
		assert.NotEmpty(t, koText, name)
		assert.NotEmpty(t, enText, name)
		assert.NotEqual(t, koText, enText, name)
		assert.Equal(t, driverFormatVerbs(koText), driverFormatVerbs(enText), name)
	}
}

// TestCurrentComposeText_FollowsSystemLocale drives the default language
// seam, tui.DetectSystemLang, which reads LANG first (S17).
func TestCurrentComposeText_FollowsSystemLocale(t *testing.T) {
	t.Setenv("LANG", "ko_KR.UTF-8")
	assert.Equal(t, "구성을 바꿀까요?", currentComposeText().Question)
	t.Setenv("LANG", "en_US.UTF-8")
	assert.Equal(t, "Change the composition?", currentComposeText().Question)
}

func driverFormatVerbs(s string) []string {
	var verbs []string
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '%' {
			verbs = append(verbs, s[i:i+2])
			i++
		}
	}
	return verbs
}
