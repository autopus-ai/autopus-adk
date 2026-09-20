// Generate frozen instruction/skill surfaces for the single-agent ablation.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

func main() {
	output := flag.String("output", "", "New empty directory for arm surfaces")
	flag.Parse()
	if *output == "" {
		panic("--output required")
	}
	for _, arm := range []string{"native", "reduced", "current"} {
		root := filepath.Join(*output, arm)
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			panic("refusing to overwrite arm surface")
		}
		if err := os.MkdirAll(root, 0700); err != nil {
			panic(err)
		}
		if arm != "native" {
			cfg := config.DefaultFullConfig("harness-benchmark")
			cfg.Platforms = []string{"codex"}
			if arm == "current" {
				cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
			}
			a := codex.NewWithRoot(root, codex.WithCLIVersion("codex-cli 0.155.1"))
			if _, err := a.Generate(context.Background(), cfg); err != nil {
				panic(err)
			}
		}
		// Runtime capabilities are controlled identically; the treatment is only
		// project instructions and published skills, not hooks or orchestration.
		if err := os.MkdirAll(filepath.Join(root, ".codex"), 0700); err != nil {
			panic(err)
		}
		runtime := `project_doc_max_bytes = 262144
web_search = "disabled"
[features]
multi_agent = false
multi_agent_v2 = false
memories = false
hooks = false
`
		if err := os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte(runtime), 0600); err != nil {
			panic(err)
		}
		fmt.Println(arm, root)
	}
}
