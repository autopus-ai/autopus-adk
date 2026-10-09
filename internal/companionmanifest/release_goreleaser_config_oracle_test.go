package companionmanifest

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type executableGoReleaserConfig struct {
	Builds []struct {
		ID      string   `yaml:"id"`
		Env     []string `yaml:"env"`
		Ldflags []string `yaml:"ldflags"`
		Hooks   struct {
			Post []struct {
				Command string   `yaml:"cmd"`
				Env     []string `yaml:"env"`
			} `yaml:"post"`
		} `yaml:"hooks"`
	} `yaml:"builds"`
	Archives []struct {
		ID    string `yaml:"id"`
		Files []struct {
			Source      string `yaml:"src"`
			Destination string `yaml:"dst"`
			StripParent *bool  `yaml:"strip_parent"`
		} `yaml:"files"`
	} `yaml:"archives"`
}

func validateProductionGoReleaserWiring(source string) error {
	var config executableGoReleaserConfig
	if err := yaml.Unmarshal([]byte(source), &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	var buildFound bool
	for _, build := range config.Builds {
		if build.ID != "auto" {
			continue
		}
		buildFound = true
		if countString(build.Env, "GOTOOLCHAIN=go1.26.9") != 1 {
			return errors.New("auto build Go toolchain pin differs")
		}
		if countString(build.Env, "GOENV=off") != 1 {
			return errors.New("auto build ambient Go env isolation differs")
		}
		if countString(build.Ldflags,
			"-X github.com/insajin/autopus-adk/internal/cli.pipelineOMPActiveStaticPolicyB64={{.Env.OMP_CONTEXT_STATIC_POLICY_B64}}") != 1 {
			return errors.New("auto build canonical static policy ldflag differs")
		}
		if err := validateCompanionPostHook(build.Hooks.Post); err != nil {
			return err
		}
	}
	if !buildFound {
		return errors.New("auto build is absent")
	}
	for _, archive := range config.Archives {
		if archive.ID == "auto" {
			return validateCompanionArchiveMappings(archive.Files)
		}
	}
	return errors.New("auto archive is absent")
}

func validateCompanionPostHook(hooks []struct {
	Command string   `yaml:"cmd"`
	Env     []string `yaml:"env"`
}) error {
	if len(hooks) != 2 || hooks[0].Command != "scripts/companion-release/verify-omp-context-release-binary.sh" ||
		hooks[1].Command != "scripts/companion-release/produce.sh" {
		return errors.New("companion producer post-hook differs")
	}
	for _, expected := range []string{
		"COMPANION_ARTIFACT={{ .Path }}", "COMPANION_PLATFORM={{ .Os }}",
		"COMPANION_ARCHITECTURE={{ .Arch }}", "COMPANION_RELEASE_TAG={{ .Env.GITHUB_REF_NAME }}",
		"OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256={{ .Env.OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256 }}",
	} {
		if countString(hooks[0].Env, expected) != 1 {
			return fmt.Errorf("OMP release binary hook environment %q differs", expected)
		}
	}
	want := []string{
		"COMPANION_ARTIFACT={{ .Path }}", "COMPANION_TARGET={{ .Target }}",
		"COMPANION_PLATFORM={{ .Os }}", "COMPANION_ARCHITECTURE={{ .Arch }}",
		"COMPANION_VERSION={{ .Version }}",
	}
	for _, expected := range want {
		if countString(hooks[1].Env, expected) != 1 {
			return fmt.Errorf("post-hook environment %q differs", expected)
		}
	}
	return nil
}

func validateCompanionArchiveMappings(files []struct {
	Source      string `yaml:"src"`
	Destination string `yaml:"dst"`
	StripParent *bool  `yaml:"strip_parent"`
}) error {
	want := map[string]string{
		"adk-companion-manifest.json":       "adk-companion-manifest.json",
		"adk-companion-manifest.sig":        "adk-companion-manifest.sig",
		"adk-companion-darwin-receipt.json": "adk-companion-darwin-receipt.json",
		archiveBundleName:                   archiveBundleName + "/**",
	}
	seen := make(map[string]int, len(want))
	for _, file := range files {
		sourceSuffix, companion := want[file.Destination]
		if !companion {
			continue
		}
		seen[file.Destination]++
		for _, token := range []string{
			`{{ if eq .Os "darwin" }}`, "dist/auto_{{ .Target }}/" + sourceSuffix,
			`scripts/companion-release/no-files-*`,
		} {
			if !strings.Contains(file.Source, token) {
				return fmt.Errorf("archive mapping %s lacks %q", file.Destination, token)
			}
		}
		if file.Destination == archiveBundleName &&
			(file.StripParent == nil || !*file.StripParent) {
			return errors.New("receipt bundle strip_parent must be true")
		}
	}
	for destination := range want {
		if seen[destination] != 1 {
			return fmt.Errorf("archive mapping %s count = %d", destination, seen[destination])
		}
	}
	return nil
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
