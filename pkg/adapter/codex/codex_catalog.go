package codex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/codexruntime"
	"github.com/insajin/autopus-adk/pkg/config"
)

const codexCatalogTimeout = 5 * time.Second

type codexRenderContext struct {
	*config.HarnessConfig
	adapter *Adapter
}

func (a *Adapter) prepareCodexCatalog(ctx context.Context) {
	if a.codexCatalogProbed {
		return
	}
	a.codexCatalogProbed = true
	output, err := codexruntime.ProbeModelCatalog(ctx, cliBinary, codexCatalogTimeout)
	if err != nil {
		a.codexCatalogJSON = nil
		return
	}
	a.codexCatalogJSON = output
}

// codexRenderData always answers with the context so a template can reach
// adapter-owned runtime facts (probed catalog, probed CLI version). Profile
// methods below fall back to the plain config values when the catalog was
// never probed, which is what the template saw before the context existed.
func (a *Adapter) codexRenderData(cfg *config.HarnessConfig) any {
	return codexRenderContext{HarnessConfig: cfg, adapter: a}
}

func (c codexRenderContext) CodexSupervisorModel() string {
	if !c.adapter.codexCatalogProbed {
		return c.HarnessConfig.CodexSupervisorModel()
	}
	return c.resolveSupervisor().Effective.Model
}

func (c codexRenderContext) CodexSupervisorEffort() string {
	if !c.adapter.codexCatalogProbed {
		return c.HarnessConfig.CodexSupervisorEffort()
	}
	return c.resolveSupervisor().Effective.Effort
}

// CodexAgentModel and CodexAgentEffort answer the agent TOML templates. Both
// return an error so a catalog that rejects the native balanced placement stops
// template execution, and with it the whole surface preparation, before any
// mapping reaches the transaction. Without a probed catalog there is nothing to
// resolve against, so both answer with the desired profile as declared.
func (c codexRenderContext) CodexAgentModel(agentName, fallbackTier, declaredEffort string) (string, error) {
	if !c.adapter.codexCatalogProbed {
		return c.HarnessConfig.CodexAgentModel(agentName, fallbackTier, declaredEffort), nil
	}
	resolution, err := c.resolveAgent(agentName, fallbackTier, declaredEffort)
	if err != nil {
		return "", err
	}
	return resolution.Effective.Model, nil
}

func (c codexRenderContext) CodexAgentEffort(agentName, fallbackTier, declaredEffort string) (string, error) {
	if !c.adapter.codexCatalogProbed {
		return c.HarnessConfig.CodexAgentEffort(agentName, fallbackTier, declaredEffort), nil
	}
	resolution, err := c.resolveAgent(agentName, fallbackTier, declaredEffort)
	if err != nil {
		return "", err
	}
	return resolution.Effective.Effort, nil
}

// resolveAgent resolves one managed agent profile against the probed catalog
// through ResolveAgentProfile, the decision the composition table shares, and
// adds only the diagnostic output. A refused placement reports nothing here:
// its error stops the template and carries the explanation.
func (c codexRenderContext) resolveAgent(
	agentName, fallbackTier, declaredEffort string,
) (config.CodexProfileResolution, error) {
	resolution, err := ResolveAgentProfile(
		c.Quality, c.adapter.codexCatalogJSON, agentName, fallbackTier, declaredEffort,
	)
	if err != nil {
		return resolution, err
	}
	if codexPlacementUnverified(resolution) {
		c.adapter.reportCodexPlacementUnverified(resolution.Requested, resolution.CatalogError)
		return resolution, nil
	}
	c.adapter.reportCodexFallback(resolution)
	return resolution, nil
}

// resolveSupervisor resolves the managed root session profile, answering with
// an empty profile when quality leaves the root model to the user.
func (c codexRenderContext) resolveSupervisor() config.CodexProfileResolution {
	resolution, managed := ResolveSupervisorProfile(c.Quality, c.adapter.codexCatalogJSON)
	if managed {
		c.adapter.reportCodexFallback(resolution)
	}
	return resolution
}

func (a *Adapter) reportCodexFallback(resolution config.CodexProfileResolution) {
	if !resolution.Fallback || a.codexFallbackWriter == nil {
		return
	}
	selected := describeCodexProfile(resolution.Effective)
	key := strings.Join([]string{
		"fallback",
		resolution.Requested.Model,
		resolution.Requested.Effort,
		selected,
		string(resolution.Reason),
	}, "|")
	if !a.claimCodexDiagnostic(key) {
		return
	}
	fmt.Fprintf(a.codexFallbackWriter,
		"Codex model fallback: requested=%s/%s selected=%s reason=%s\n",
		resolution.Requested.Model,
		resolution.Requested.Effort,
		selected,
		resolution.Reason,
	)
}

// claimCodexDiagnostic reports whether this diagnostic is new. Sixteen agent
// templates each ask for a model and an effort, so the same profile is resolved
// many times per run and must still be reported once.
func (a *Adapter) claimCodexDiagnostic(key string) bool {
	if a.codexFallbackSeen == nil {
		a.codexFallbackSeen = make(map[string]struct{})
	}
	if _, exists := a.codexFallbackSeen[key]; exists {
		return false
	}
	a.codexFallbackSeen[key] = struct{}{}
	return true
}
