package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/version"
)

type pluginManifest struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      pluginAuthor    `json:"author"`
	Homepage    string          `json:"homepage"`
	Repository  string          `json:"repository"`
	License     string          `json:"license"`
	Keywords    []string        `json:"keywords"`
	Skills      string          `json:"skills"`
	Interface   pluginInterface `json:"interface"`
}

type pluginAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	URL   string `json:"url"`
}

type pluginInterface struct {
	DisplayName       string   `json:"displayName"`
	ShortDescription  string   `json:"shortDescription"`
	LongDescription   string   `json:"longDescription"`
	DeveloperName     string   `json:"developerName"`
	Category          string   `json:"category"`
	Capabilities      []string `json:"capabilities"`
	WebsiteURL        string   `json:"websiteURL"`
	PrivacyPolicyURL  string   `json:"privacyPolicyURL"`
	TermsOfServiceURL string   `json:"termsOfServiceURL"`
	DefaultPrompt     []string `json:"defaultPrompt"`
	BrandColor        string   `json:"brandColor"`
}

type marketplaceDoc struct {
	Name      string             `json:"name"`
	Interface marketplaceUI      `json:"interface,omitempty"`
	Plugins   []marketplaceEntry `json:"plugins"`
}

type marketplaceUI struct {
	DisplayName string `json:"displayName,omitempty"`
}

type marketplaceEntry struct {
	Name     string            `json:"name"`
	Source   marketplaceSource `json:"source"`
	Policy   marketplacePolicy `json:"policy"`
	Category string            `json:"category"`
}

type marketplaceSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type marketplacePolicy struct {
	Installation   string   `json:"installation"`
	Authentication string   `json:"authentication"`
	Products       []string `json:"products,omitempty"`
}

var codexPluginSemverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func (a *Adapter) renderPluginManifestJSON(cfg *config.HarnessConfig, routerContent string) (string, error) {
	doc := pluginManifest{
		Name:        "auto",
		Version:     codexPluginVersion(a.pluginBuildVersion(), cfg, routerContent),
		Description: "Autopus workflow router for Codex: setup, status, goal, update, plan, go, fix, review, sync, idea, map, why, verify, secure, test, qa, dev, canary, and doctor.",
		Author:      pluginAuthor{Name: "Autopus", Email: "noreply@autopus.co", URL: "https://autopus.co"},
		Homepage:    "https://autopus.co",
		Repository:  "https://github.com/insajin/autopus-adk",
		License:     "Apache-2.0",
		Keywords:    []string{"autopus", "workflow", "planning", "codex", "multi-provider"},
		Skills:      "./skills",
		Interface: pluginInterface{
			DisplayName:       "Auto",
			ShortDescription:  "Autopus workflow router for Codex",
			LongDescription:   "Run the full Autopus setup/status/goal/update/plan/go/fix/review/sync/idea/map/why/verify/secure/test/qa/dev/canary/doctor workflow set from Codex with a local plugin plus repository-managed helper docs.",
			DeveloperName:     "Autopus",
			Category:          "Developer Tools",
			Capabilities:      []string{"Interactive", "Write", "Planning"},
			WebsiteURL:        "https://autopus.co",
			PrivacyPolicyURL:  "https://autopus.co/privacy",
			TermsOfServiceURL: "https://autopus.co/terms",
			DefaultPrompt: []string{
				"@auto plan 요구사항을 SPEC으로 정리해줘",
				"@auto go SPEC-EXAMPLE-001",
				"@auto review",
			},
			BrandColor: "#0F766E",
		},
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("plugin.json 직렬화 실패: %w", err)
	}
	return string(data) + "\n", nil
}

// pluginBuildVersion is the raw version the plugin manifest derives its base
// from: the pinned generator version when one is set, else the build version.
func (a *Adapter) pluginBuildVersion() string {
	if a.pluginBaseVersion != nil {
		return *a.pluginBaseVersion
	}
	return version.Version()
}

func codexPluginVersion(rawVersion string, cfg *config.HarnessConfig, routerContent string) string {
	base := codexPluginBaseVersion(rawVersion)
	project := "local"
	if cfg != nil && strings.TrimSpace(cfg.ProjectName) != "" {
		project = cfg.ProjectName
	}
	slug := codexPluginSlug(project)
	cacheKey := checksum(codexPluginCacheInput(cfg, routerContent))
	if len(cacheKey) > 12 {
		cacheKey = cacheKey[:12]
	}
	return fmt.Sprintf("%s+codex.%s.%s", base, slug, cacheKey)
}

func codexPluginBaseVersion(raw string) string {
	base := strings.TrimSpace(strings.TrimPrefix(raw, "v"))
	if idx := strings.Index(base, "+"); idx >= 0 {
		base = base[:idx]
	}
	if codexPluginSemverRe.MatchString(base) {
		return base
	}
	return "0.0.0-dev"
}

func codexPluginSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "local"
	}
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	if slug == "" {
		return "local"
	}
	return slug
}

func codexPluginCacheInput(cfg *config.HarnessConfig, routerContent string) string {
	parts := []string{"codex-plugin", routerContent}
	if cfg != nil {
		parts = append(parts,
			cfg.ProjectName,
			strings.Join(cfg.Platforms, ","),
			cfg.Skills.SharedSurface,
			cfg.Skills.Compiler.Mode,
			strings.Join(cfg.Skills.Compiler.Bundles, ","),
			strings.Join(cfg.Skills.Compiler.ExplicitSkills, ","),
			cfg.Skills.Compiler.CodexLongTailTarget,
		)
	}
	return strings.Join(parts, "\x00")
}

func (a *Adapter) renderMarketplaceJSON() (string, error) {
	doc := marketplaceDoc{
		Name:      "autopus-local",
		Interface: marketplaceUI{DisplayName: "Autopus Local"},
		Plugins: []marketplaceEntry{{
			Name:     "auto",
			Source:   marketplaceSource{Source: "local", Path: "./.autopus/plugins/auto"},
			Policy:   marketplacePolicy{Installation: "AVAILABLE", Authentication: "ON_INSTALL"},
			Category: "Developer Tools",
		}},
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marketplace.json 직렬화 실패: %w", err)
	}
	existingPath := filepath.Join(a.root, ".agents", "plugins", "marketplace.json")
	if existing, readErr := os.ReadFile(existingPath); readErr == nil {
		data, err = mergeCodexMarketplace(existing, data)
		if err != nil {
			return "", err
		}
	} else if !os.IsNotExist(readErr) {
		return "", fmt.Errorf("existing marketplace.json 읽기 실패: %w", readErr)
	}
	return string(data) + "\n", nil
}
