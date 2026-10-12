package cli

// SPEC-MODELUX-002 T5 (Form Text): every form string ships in both
// languages with the same format verbs, and only ko reads Korean.

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposeFormText_EveryStringIsTranslated(t *testing.T) {
	ko := reflect.ValueOf(composeFormTextFor("ko"))
	en := reflect.ValueOf(composeFormTextFor("en"))
	for i := range ko.NumField() {
		name := ko.Type().Field(i).Name
		koText, enText := ko.Field(i).String(), en.Field(i).String()
		assert.NotEmpty(t, koText, name)
		assert.NotEmpty(t, enText, name)
		assert.Equal(t, driverFormatVerbs(koText), driverFormatVerbs(enText), name)
		if name != "OMPThinking" {
			assert.NotEqual(t, koText, enText, name)
		}
	}
}

func TestComposeFormTextFor_KoreanOnlyForKo(t *testing.T) {
	assert.Equal(t, "구성을 바꿀까요?", composeFormTextFor("ko").Menu)
	for _, lang := range []string{"en", "ja", "zh", ""} {
		assert.Equal(t, "Change the composition?", composeFormTextFor(lang).Menu, "lang %q", lang)
	}
}

// The tool tab's change lines follow the same rule.
func TestComposeToolText_EveryStringIsTranslated(t *testing.T) {
	ko := reflect.ValueOf(composeToolTextFor("ko"))
	en := reflect.ValueOf(composeToolTextFor("en"))
	for i := range ko.NumField() {
		name := ko.Type().Field(i).Name
		koText, enText := ko.Field(i).String(), en.Field(i).String()
		assert.NotEmpty(t, koText, name)
		assert.Equal(t, driverFormatVerbs(koText), driverFormatVerbs(enText), name)
	}
	assert.Equal(t, composeToolTextFor("en"), composeToolTextFor("ja"))
}
