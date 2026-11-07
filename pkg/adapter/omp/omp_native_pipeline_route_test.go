package omp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestOMPNativePipelineRoute_MarkdownAutoOwnsInteractiveAndExplicitRouteOwnsHeadless(t *testing.T) {
	root := t.TempDir()
	cfg := optedInOMPContextBridgeConfig()
	require.NoError(t, config.Save(root, cfg))
	generated, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	path := filepath.Join(root, ".omp", "extensions", "autopus-pipeline.ts")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	source := string(body)
	require.NotContains(t, source, `registerCommand("auto"`)
	require.Equal(t, 1, countLiteral(source, `registerCommand("autopus-pipeline"`))
	require.Contains(t, source, `Usage: /autopus-pipeline go SPEC-ID`)
	require.Contains(t, source, `[--execution-owner omp|orca]`)
	require.Contains(t, source, `["--execution-owner", new Set(["omp", "orca"])]`)
	require.NotContains(t, source, `"OMP"`)
	require.NotContains(t, source, `"local"`)
	require.NotContains(t, source, `"supervised"`)
	require.Contains(t, source, `process.env.AUTOPUS_OMP_MANAGED_INNER === "1"`)
	require.Equal(t, 1, countLiteral(source, `["pipeline", "run", specID, "--platform", "omp"]`))
	require.Equal(t, 1, countLiteral(source, `spawn("auto", argv`))
	require.NotContains(t, source, `"parallel"`)
	require.Contains(t, source, "shell: false")
	require.NotContains(t, source, "promotion")
	require.NotContains(t, source, "history_rows")
	require.NotContains(t, source, "capabilities")
	require.NotContains(t, source, "exec(")
	require.NotContains(t, source, "sh -c")

	autoPath := filepath.Join(root, ".omp", "commands", "auto.md")
	autoBody, err := os.ReadFile(autoPath)
	require.NoError(t, err)
	require.Equal(t, 0, countLiteral(string(autoBody), `spawn(`),
		"interactive /auto must dispatch through the current OMP session without spawning a child")
	require.Equal(t, 0, countLiteral(string(autoBody), `["pipeline", "run"`),
		"interactive /auto must not enter the headless RPC backend")
	require.Len(t, commandFileNames(t, root), len(workflowSpecs))

	var headlessCallOwners []string
	nativeAutoRegistrations := 0
	for _, file := range generated.Files {
		content := string(file.Content)
		nativeAutoRegistrations += strings.Count(content, `registerCommand("auto"`)
		if strings.Contains(content, `["pipeline", "run", specID, "--platform", "omp"]`) {
			headlessCallOwners = append(headlessCallOwners, filepath.ToSlash(file.TargetPath))
		}
	}
	require.Zero(t, nativeAutoRegistrations,
		".omp/commands/auto.md must remain the only interactive /auto owner")
	require.Equal(t, []string{ompNativePipelineRouteTarget}, headlessCallOwners,
		"only the explicit /autopus-pipeline extension may invoke the headless OMP backend")

	require.NoError(t, NewWithRoot(root).Clean(context.Background()))
	require.NoFileExists(t, path)
}

func TestOMPNativePipelineRoute_IdentityIsGeneratedSourceOfTruth(t *testing.T) {
	identity := ExpectedOMPNativePipelineRouteSourceIdentity()
	require.Equal(t, ".omp/extensions/autopus-pipeline.ts", identity.TargetPath)
	require.Regexp(t, `^[0-9a-f]{64}$`, identity.SHA256)
	require.Positive(t, identity.Size)

	bridge := ExpectedOMPContextBridgeSourceIdentity()
	require.Equal(t, ".omp/extensions/autopus-context.ts", bridge.TargetPath)
	require.Regexp(t, `^[0-9a-f]{64}$`, bridge.SHA256)
	require.Positive(t, bridge.Size)
}

func TestOMPNativePipelineRoute_RegistersExplicitCommandOnlyOutsideManagedInnerRuntime(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is required to execute the generated TypeScript route contract")
	}
	harness := `
let registrations = 0;
let registeredName = "";
const api = { registerCommand(name: string) { registrations++; registeredName = name; } };
delete process.env.AUTOPUS_OMP_MANAGED_INNER;
autopusPipelineRoute(api as any);
if (registrations !== 1) throw new Error("outer route was not registered exactly once");
if (registeredName !== "autopus-pipeline") throw new Error("outer route claimed interactive /auto");
const sequential = parseRoute("go SPEC-TEST-001 --strategy sequential");
if (!sequential.ok) throw new Error("sequential headless strategy was rejected");
const parallel = parseRoute("go SPEC-TEST-001 --strategy parallel");
if (parallel.ok) throw new Error("parallel remained in the headless allowlist");
const ompOwner = parseRoute("go SPEC-TEST-001 --execution-owner omp");
if (!ompOwner.ok || ompOwner.forwardedFlags.join(" ") !== "--execution-owner omp") throw new Error("exact omp owner was not forwarded");
const orcaOwner = parseRoute("go SPEC-TEST-001 --execution-owner orca");
if (!orcaOwner.ok || orcaOwner.forwardedFlags.join(" ") !== "--execution-owner orca") throw new Error("exact orca owner was not forwarded");
for (const invalidOwner of ["OMP", "Orca", "omp,orca", "local", "supervised"]) {
  if (parseRoute("go SPEC-TEST-001 --execution-owner " + invalidOwner).ok) throw new Error("invalid owner was accepted: " + invalidOwner);
}
const mixedOwner = parseRoute("go SPEC-TEST-001 --execution-owner omp --execution-owner orca");
if (mixedOwner.ok) throw new Error("mixed owner selection was accepted");
registrations = 0;
process.env.AUTOPUS_OMP_MANAGED_INNER = "1";
autopusPipelineRoute(api as any);
if (registrations !== 0) throw new Error("managed inner route registered recursively");
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bun, "run", "-")
	cmd.Stdin = strings.NewReader(ompNativePipelineRouteSource + harness)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

func countLiteral(source, literal string) int {
	count := 0
	for {
		index := len(source)
		for i := 0; i+len(literal) <= len(source); i++ {
			if source[i:i+len(literal)] == literal {
				index = i
				break
			}
		}
		if index == len(source) {
			return count
		}
		count++
		source = source[index+len(literal):]
	}
}
