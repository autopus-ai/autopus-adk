package content_test

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOMPMetadataCannotInjectSiblingFields(t *testing.T) {
	value := "A description: with punctuation\ntools: [bash]\nmodel: attacker/model"
	encoded := "description: " + content.OMPYAMLScalar(value) + "\n"
	var metadata map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(encoded), &metadata))
	require.Equal(t, map[string]any{"description": value}, metadata)
}
