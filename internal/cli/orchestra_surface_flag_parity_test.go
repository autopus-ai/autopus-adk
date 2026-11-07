package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	orchestraInvocationPattern = regexp.MustCompile(`auto orchestra ([a-z][a-z-]*)([^\n]*)`)
	orchestraFlagPattern       = regexp.MustCompile(`--([a-z][a-z0-9-]*)`)
)

// TestOrchestraGeneratedSurfaceFlagsExistOnCLI prevents generated provider
// guidance from requiring a flag that the released auto binary cannot parse.
func TestOrchestraGeneratedSurfaceFlagsExistOnCLI(t *testing.T) {
	t.Parallel()

	root := newOrchestraCmd()
	commands := make(map[string]*cobra.Command)
	for _, command := range root.Commands() {
		commands[command.Name()] = command
	}

	moduleRoot := filepath.Join("..", "..")
	surfaceRoots := []string{
		filepath.Join(moduleRoot, "content", "skills"),
		filepath.Join(moduleRoot, "templates", "claude"),
		filepath.Join(moduleRoot, "templates", "codex"),
		filepath.Join(moduleRoot, "templates", "gemini"),
	}
	foundInvocations := 0
	// content/skills no longer ships a skill that runs auto orchestra, so
	// only the total is required to be positive.
	for _, surfaceRoot := range surfaceRoots {
		err := filepath.WalkDir(surfaceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || (!strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".tmpl")) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, invocation := range orchestraInvocationPattern.FindAllStringSubmatch(string(data), -1) {
				foundInvocations++
				commandName := invocation[1]
				command, ok := commands[commandName]
				require.Truef(t, ok, "%s references unknown orchestra command %q", path, commandName)
				if !ok {
					continue
				}
				for _, flag := range orchestraFlagPattern.FindAllStringSubmatch(invocation[2], -1) {
					assert.NotNilf(t, command.Flags().Lookup(flag[1]),
						"%s requires unsupported `auto orchestra %s --%s`", path, commandName, flag[1])
				}
			}
			return nil
		})
		require.NoError(t, err)
	}
	require.Positive(t, foundInvocations, "expected generated orchestra invocations to be scanned")
}
