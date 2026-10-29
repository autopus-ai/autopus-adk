package omp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompHubMentionRe finds guidance that sends the session to the hub tool:
// the tool itself, a hub call payload, or a bare hub operation.
var ompHubMentionRe = regexp.MustCompile("`hub`|hub with \\{|\\bhub (?:send|wait|cancel|list|jobs|message|job wait|cancellation)\\b")

// omp/18.3.0 replaced the hub tool with wait and proc:// / agent:// URLs, and
// the CI-pinned omp/17.2.7 has only hub. Generated guidance carries no
// version branch, so a line that names hub must also name the 18.3+ form, or
// an 18.3+ session is told to call a tool it does not have.
func TestGeneratedOMPGuidancePairsHubWithItsReplacement(t *testing.T) {
	for name, cfg := range map[string]*config.HarnessConfig{
		"autopus agents": configForOMP(),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, config.Save(root, cfg))
			_, err := NewWithRoot(root).Generate(context.Background(), cfg)
			require.NoError(t, err)

			mentions := 0
			walkErr := filepath.WalkDir(filepath.Join(root, ".omp"), func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() || filepath.Ext(path) != ".md" {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(root, path)
				for number, line := range strings.Split(string(data), "\n") {
					if !ompHubMentionRe.MatchString(line) {
						continue
					}
					mentions++
					assert.Regexp(t, "proc://|agent://|`wait`|\\bwait \\(OMP 18\\.3\\+\\)", line,
						"%s:%d names hub without its OMP 18.3+ form", rel, number+1)
				}
				return nil
			})
			require.NoError(t, walkErr)
			assert.NotZero(t, mentions, "the older-OMP hub form must stay for the pinned omp/17.2.7")
		})
	}
}
