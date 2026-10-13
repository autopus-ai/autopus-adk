package omp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

const expectedOMPContextBridgeSource = `type BridgeEvent = "pre_compaction" | "post_compaction";

type BridgeContext = {
  ui: {
    confirm(title: string, message: string): boolean | Promise<boolean>;
  };
};

type BridgeHashes = {
  binding_hash: string;
  options_hash: string;
  session_hash: string;
  nonce_hash: string;
};

type BridgeConfig = {
  hashes: BridgeHashes;
  ackTimeoutMs: number;
};

type PreCompactResult = {
  cancel: true;
};

type BridgeAPI = {
  on(
    event: "session_before_compact",
    handler: (
      _event: unknown,
      context: BridgeContext,
    ) => PreCompactResult | undefined | Promise<PreCompactResult | undefined>,
  ): void;
  on(
    event: "session_compact",
    handler: (_event: unknown, context: BridgeContext) => void | Promise<void>,
  ): void;
};

const HASH_PATTERN = /^sha256:[0-9a-f]{64}$/;
const INTEGER_PATTERN = /^[0-9]+$/;
const DEFAULT_ACK_TIMEOUT_MS = 5000;
const MIN_ACK_TIMEOUT_MS = 50;
const MAX_ACK_TIMEOUT_MS = 10000;

function checkedHash(value: string | undefined): string | null {
  return value !== undefined && HASH_PATTERN.test(value) ? value : null;
}

function readACKTimeout(): number | null {
  const value = process.env.AUTOPUS_OMP_CONTEXT_ACK_TIMEOUT_MS;
  if (value === undefined) {
    return DEFAULT_ACK_TIMEOUT_MS;
  }
  if (!INTEGER_PATTERN.test(value)) {
    return null;
  }
  const timeout = Number(value);
  return Number.isSafeInteger(timeout) && timeout >= MIN_ACK_TIMEOUT_MS && timeout <= MAX_ACK_TIMEOUT_MS
    ? timeout
    : null;
}

function readBridgeConfig(): BridgeConfig | null {
  const bindingHash = checkedHash(process.env.AUTOPUS_OMP_CONTEXT_BINDING_HASH);
  const optionsHash = checkedHash(process.env.AUTOPUS_OMP_CONTEXT_OPTIONS_HASH);
  const sessionHash = checkedHash(process.env.AUTOPUS_OMP_CONTEXT_SESSION_HASH);
  const nonceHash = checkedHash(process.env.AUTOPUS_OMP_CONTEXT_NONCE_HASH);
  const ackTimeoutMs = readACKTimeout();
  if (bindingHash === null || optionsHash === null || sessionHash === null ||
      nonceHash === null || ackTimeoutMs === null) {
    return null;
  }
  return {
    hashes: {
      binding_hash: bindingHash,
      options_hash: optionsHash,
      session_hash: sessionHash,
      nonce_hash: nonceHash,
    },
    ackTimeoutMs,
  };
}

async function confirmSafely(
  context: BridgeContext,
  event: BridgeEvent,
  hashes: BridgeHashes,
  ackTimeoutMs: number,
): Promise<boolean> {
  const message = JSON.stringify({
    schema_version: "autopus.omp-context-bridge.v1",
    event,
    ...hashes,
  });
  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = (confirmed: boolean): void => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      resolve(confirmed);
    };
    const timer = setTimeout(() => finish(false), ackTimeoutMs);
    try {
      if (typeof context.ui.confirm !== "function") {
        finish(false);
        return;
      }
      Promise.resolve(context.ui.confirm("Autopus context " + event, message)).then(
        (confirmed) => finish(confirmed === true),
        () => finish(false),
      );
    } catch {
      finish(false);
    }
  });
}

export default function autopusContextBridge(pi: BridgeAPI): void {
  const config = readBridgeConfig();
  pi.on("session_before_compact", async (_event, context) => {
    if (config === null || !(await confirmSafely(
      context,
      "pre_compaction",
      config.hashes,
      config.ackTimeoutMs,
    ))) {
      return { cancel: true } as const;
    }
    return undefined;
  });
  if (config === null) {
    return;
  }
  pi.on("session_compact", async (_event, context) => {
    await confirmSafely(context, "post_compaction", config.hashes, config.ackTimeoutMs);
  });
}
`

func TestOMPContextBridge_NoOptInPreservesExactPreparedFiles(t *testing.T) {
	adapterUnderTest := NewWithRoot(t.TempDir())
	baseline := nativeConfigForOMP()
	baselineFiles, err := adapterUnderTest.prepareFiles(context.Background(), baseline)
	require.NoError(t, err)

	catalogOnly := nativeConfigForOMP()
	catalogOnly.OMPContextPolicy.Profiles = map[string]config.OMPContextProfileConf{
		"bridge": {},
	}
	catalogFiles, err := adapterUnderTest.prepareFiles(context.Background(), catalogOnly)
	require.NoError(t, err)

	// Merely defining a context profile must leave the current generated
	// surface byte-identical; unrelated prose changes do not alter this invariant.
	assert.Equal(t, fingerprintOMPFileMappings(t, baselineFiles), fingerprintOMPFileMappings(t, catalogFiles))
	assert.NotContains(t, ompMappingTargets(baselineFiles), ".omp/extensions/autopus-context.ts")
	assert.NotContains(t, ompMappingTargets(baselineFiles), ompNativePipelineRouteTarget)
}

func TestOMPContextBridge_OptInEmitsExactBodyFreeMapping(t *testing.T) {
	cfg := optedInOMPContextBridgeConfig()
	files, err := NewWithRoot(t.TempDir()).prepareFiles(context.Background(), cfg)
	require.NoError(t, err)

	var bridgeMappings []adapter.FileMapping
	for _, file := range files {
		if file.TargetPath == ".omp/extensions/autopus-context.ts" {
			bridgeMappings = append(bridgeMappings, file)
		}
	}
	require.Len(t, bridgeMappings, 1)
	bridge := bridgeMappings[0]
	assert.Equal(t, adapter.OverwriteAlways, bridge.OverwritePolicy)
	assert.Equal(t, adapter.Checksum(expectedOMPContextBridgeSource), bridge.Checksum)
	assert.Equal(t, expectedOMPContextBridgeSource, string(bridge.Content))

	for _, forbidden := range []string{
		"branchEntries", "compactionEntry", "customInstructions", "preparation",
		"sessionManager", "getSessionId", "sendMessage", "appendEntry",
		"registerProvider", "registerTool", "fetch(", "node:fs", "Deno.", "Bun.",
		"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "/Users/", "../",
	} {
		assert.NotContains(t, string(bridge.Content), forbidden)
	}
	assert.Equal(t, 1, strings.Count(string(bridge.Content), "process.env.AUTOPUS_OMP_CONTEXT_BINDING_HASH"))
	assert.Equal(t, 1, strings.Count(string(bridge.Content), "process.env.AUTOPUS_OMP_CONTEXT_OPTIONS_HASH"))
	assert.Equal(t, 1, strings.Count(string(bridge.Content), "process.env.AUTOPUS_OMP_CONTEXT_SESSION_HASH"))
	assert.Equal(t, 1, strings.Count(string(bridge.Content), "process.env.AUTOPUS_OMP_CONTEXT_NONCE_HASH"))
	assert.Equal(t, 1, strings.Count(string(bridge.Content), "process.env.AUTOPUS_OMP_CONTEXT_ACK_TIMEOUT_MS"))
	assert.Contains(t, string(bridge.Content), "context.ui.confirm(\"Autopus context \" + event, message)")
	assert.Contains(t, string(bridge.Content), "return { cancel: true } as const;")
	assert.Contains(t, string(bridge.Content), "const DEFAULT_ACK_TIMEOUT_MS = 5000;")
	assert.Contains(t, string(bridge.Content), "const MIN_ACK_TIMEOUT_MS = 50;")
	assert.Contains(t, string(bridge.Content), "const MAX_ACK_TIMEOUT_MS = 10000;")
	assert.NotContains(t, string(bridge.Content), "notify(")
	assert.NotContains(t, string(bridge.Content), "} finally {")
	assert.False(t, NewWithRoot(t.TempDir()).SupportsHooks(), "generic hook semantics remain unsupported")
}

func optedInOMPContextBridgeConfig() *config.HarnessConfig {
	cfg := configForOMP()
	cfg.OMPContextPolicy = config.OMPContextPolicyConf{
		Profile: "bridge",
		Profiles: map[string]config.OMPContextProfileConf{
			"bridge": {},
		},
	}
	return cfg
}

func fingerprintOMPFileMappings(t *testing.T, files []adapter.FileMapping) string {
	t.Helper()
	type exactMapping struct {
		SourceTemplate  string                  `json:"source_template"`
		TargetPath      string                  `json:"target_path"`
		OverwritePolicy adapter.OverwritePolicy `json:"overwrite_policy"`
		Checksum        string                  `json:"checksum"`
		Content         string                  `json:"content"`
	}
	exact := make([]exactMapping, 0, len(files))
	for _, file := range files {
		exact = append(exact, exactMapping{
			SourceTemplate: file.SourceTemplate, TargetPath: file.TargetPath,
			OverwritePolicy: file.OverwritePolicy, Checksum: file.Checksum,
			Content: string(file.Content),
		})
	}
	sort.Slice(exact, func(i, j int) bool { return exact[i].TargetPath < exact[j].TargetPath })
	encoded, err := json.Marshal(exact)
	require.NoError(t, err)
	return adapter.Checksum(string(encoded))
}

func ompMappingTargets(files []adapter.FileMapping) []string {
	targets := make([]string, 0, len(files))
	for _, file := range files {
		targets = append(targets, file.TargetPath)
	}
	sort.Strings(targets)
	return targets
}
