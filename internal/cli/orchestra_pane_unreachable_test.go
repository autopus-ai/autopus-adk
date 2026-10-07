package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-PANERM-001 REQ-16, REQ-17, REQ-18: the orchestra pane backend is
// retired. The guard keeps its identifiers (group I), its files (group P), its
// terminal coupling, and its instruction tokens from coming back.

// paneGroupIdentifiers is the spec.md group I working list. The names are
// distinctive, so a declaration or reference fails in every scanned package.
var paneGroupIdentifiers = []string{
	"RunPaneOrchestra", "RunPaneOrchestraDetached", "RunInteractivePaneOrchestra", "runInteractiveDebate",
	"NewInteractivePaneBackend", "InteractivePaneBackend", "paneCapable", "paneLaunchFor",
	"buildPaneLaunchCommand", "HookSession", "ShouldDetach", "ScreenPollDetector", "FileIPCDetector",
	"PaneArgs", "InteractiveInput", "WorkingPatterns",
}

// paneDeletedExports is research.md "Group I Final List (T7)": the exported
// declarations of the deleted pkg/orchestra code. Some are generic (Job,
// WarmPool), so internal/cli matches them only as orchestra.<name>.
var paneDeletedExports = []string{
	"BuildYieldOutput", "CleanRoundSignals", "CleanScreenForCrossPollination", "CleanupStaleJobs",
	"CompletionDetector", "CompletionPattern", "DefaultCompletionPatterns", "DefaultHookProviders",
	"DefaultPromptPatterns", "DefaultStartupHookProviders", "HookInput", "HookResult",
	"HookResultToProviderResponse", "IdleThreshold", "Job", "JobStatus", "JobStatusDone", "JobStatusError",
	"JobStatusPartial", "JobStatusRunning", "JobStatusTimeout", "LoadJob", "LoadSession",
	"NewCompletionDetector", "NewCompletionDetectorWithConfig", "NewHookSession", "NewSignalEmitter",
	"NewSurfaceManager", "NewWarmPool", "OrchestraSession", "ReapOrphanSurfaces", "RemoveSession",
	"ResolveSessionTerminal", "ResolveSessionTerminalWithWorkspace", "RoundSignalName", "SaveSession",
	"SendRoundEnvToPane", "SendSessionEnvToPane", "SessionProviderConfig", "SessionProviderResponse",
	"SessionReadyPatterns", "SetRoundEnv", "SignalDetector", "SignalEmitter", "SurfaceManager",
	"UpdateSession", "WaitAndCollectHookResults", "WarmPool",
}

// paneSessionEnv is the group I variable that only group P code set.
const paneSessionEnv = "AUTOPUS_SESSION_ID"

const (
	paneOrchestraImport = "github.com/insajin/autopus-adk/pkg/orchestra"
	paneTerminalImport  = "github.com/insajin/autopus-adk/pkg/terminal"
)

// paneSurfaceCalls are the terminal calls the pane backend used to type
// prompts, answer permission prompts, and read provider screens (S15).
var paneSurfaceCalls = map[string]bool{"SendCommand": true, "SendLongText": true, "ReadScreen": true}

// paneGroupFiles are the spec.md group P patterns. A test file matches
// through the name of its production counterpart.
var paneGroupFiles = []string{
	"pane_*.go", "interactive*.go", "hook_*.go", "completion_poll.go", "completion_file_ipc.go",
	"completion_signal.go", "completion_detector.go", "cc21_monitor.go", "signal_emitter.go",
	"round_signal.go", "surface_manager.go", "surface_tracker*.go", "warm_pool.go", "read_screen.go",
	"screen_sanitizer.go", "relay_pane.go", "recovery_hook_launch.go", "session*.go", "yield_session.go",
	"reviewer_response_file.go", "detach.go", "job.go",
}

// retiredInstructionTokens are the retired flags, subcommands, and group K
// keys that the CHANGELOG entry names (REQ-16, REQ-18).
var retiredInstructionTokens = []string{
	"--no-detach", "--subprocess", "--plain", "--yield-rounds",
	"auto orchestra collect", "auto orchestra inject", "auto orchestra cleanup",
	"auto orchestra status", "auto orchestra wait", "auto orchestra result",
	"pane_args", "interactive_input", "working_patterns", "subprocess.enabled", "monitor_pattern_timeout_ms",
}

// paneGuardException lets one instruction file keep one retired token; an
// entry names the path, the token, and the reason.
type paneGuardException struct{ path, token, reason string }

// paneGuardAllowlist is empty at merge (spec.md Compatibility Contract).
var paneGuardAllowlist []paneGuardException

type paneGoScope int

const (
	paneScopeOrchestra paneGoScope = iota // bare identifiers and terminal coupling
	paneScopeCLI                          // deleted exports only as orchestra.<name>
)

// The pane backend is retired: no production file may declare or reference
// its identifiers, pkg/orchestra may not regain its files or drive a terminal,
// and shipped instruction sources may not mention a retired token.
func TestCLIProductionCodeNeverCallsPaneEntryPoints(t *testing.T) {
	root := filepath.Join("..", "..")
	t.Run("orchestra_identifiers_and_terminal", func(t *testing.T) {
		assert.Empty(t, scanPaneGoFiles(t, root, "pkg/orchestra", paneScopeOrchestra))
	})
	t.Run("cli_identifiers", func(t *testing.T) {
		assert.Empty(t, scanPaneGoFiles(t, root, "internal/cli", paneScopeCLI))
	})
	t.Run("group_P_files", func(t *testing.T) {
		assert.Empty(t, paneGroupFileFindings(t, root))
	})
	t.Run("instruction_tokens", func(t *testing.T) {
		assert.Empty(t, retiredTokenFindings(t, root, paneGuardAllowlist))
	})
}

// scanPaneGoFiles reports group I declarations and references, the session
// variable, and for pkg/orchestra the pkg/terminal import and pane surface
// calls, in every non-test Go file under root/dir outside testdata.
func scanPaneGoFiles(t *testing.T, root, dir string, scope paneGoScope) []string {
	t.Helper()
	idents, exports := paneNameSet(paneGroupIdentifiers), paneNameSet(paneDeletedExports)
	fset := token.NewFileSet()
	var findings []string
	err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, aliases := paneGuardRel(t, root, path), paneOrchestraAliases(file)
		report := func(pos token.Pos, what string) {
			findings = append(findings, fmt.Sprintf("%s:%d: %s", rel, fset.Position(pos).Line, what))
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ImportSpec:
				if p, _ := strconv.Unquote(n.Path.Value); scope == paneScopeOrchestra && p == paneTerminalImport {
					report(n.Pos(), "imports "+strconv.Quote(p))
				}
				return false
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && scope == paneScopeOrchestra && paneSurfaceCalls[sel.Sel.Name] {
					report(sel.Sel.Pos(), "pane surface call "+strconv.Quote("."+sel.Sel.Name+"("))
				}
			case *ast.SelectorExpr:
				if x, ok := n.X.(*ast.Ident); ok && scope == paneScopeCLI && aliases[x.Name] && exports[n.Sel.Name] {
					report(n.Sel.Pos(), "group I identifier "+strconv.Quote(n.Sel.Name))
				}
			case *ast.Ident:
				if idents[n.Name] || (scope == paneScopeOrchestra && exports[n.Name]) {
					report(n.Pos(), "group I identifier "+strconv.Quote(n.Name))
				}
			case *ast.BasicLit:
				if n.Kind == token.STRING && strings.Contains(n.Value, paneSessionEnv) {
					report(n.Pos(), "group I identifier "+strconv.Quote(paneSessionEnv))
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	return findings
}

// paneGroupFileFindings reports every file under root/pkg/orchestra whose
// name, or whose production counterpart's name, matches a group P pattern.
func paneGroupFileFindings(t *testing.T, root string) []string {
	t.Helper()
	var findings []string
	err := filepath.WalkDir(filepath.Join(root, "pkg", "orchestra"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return err
		}
		name := d.Name()
		if base, ok := strings.CutSuffix(name, "_test.go"); ok {
			name = base + ".go"
		}
		for _, pattern := range paneGroupFiles {
			if ok, _ := filepath.Match(pattern, name); ok {
				findings = append(findings, fmt.Sprintf("%s: group P file %q", paneGuardRel(t, root, path), pattern))
				break
			}
		}
		return nil
	})
	require.NoError(t, err)
	return findings
}

// retiredTokenFindings reports `<file>:<line>: retired token "<token>"` for
// every retired token under root/content, root/templates, and root/configs
// that no allowlist entry covers. A missing root fails the scan.
func retiredTokenFindings(t *testing.T, root string, allow []paneGuardException) []string {
	t.Helper()
	allowed := map[[2]string]bool{}
	for _, entry := range allow {
		require.NotEmpty(t, entry.reason, "allowlist entry %s %q needs a reason", entry.path, entry.token)
		allowed[[2]string{entry.path, entry.token}] = true
	}
	var findings []string
	for _, dir := range []string{"content", "templates", "configs"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel := paneGuardRel(t, root, path)
			for i, line := range strings.Split(string(data), "\n") {
				for _, tok := range retiredTokensIn(line) {
					if !allowed[[2]string{rel, tok}] {
						findings = append(findings, fmt.Sprintf("%s:%d: retired token %q", rel, i+1, tok))
					}
				}
			}
			return nil
		})
		require.NoError(t, err)
	}
	return findings
}

// retiredTokensIn lists the retired tokens that occur in line as whole words:
// neither neighbor is a letter, digit, '_', or '-'.
func retiredTokensIn(line string) []string {
	var found []string
	for _, tok := range retiredInstructionTokens {
		for from := 0; from < len(line); {
			i := strings.Index(line[from:], tok)
			if i < 0 {
				break
			}
			start, end := from+i, from+i+len(tok)
			if !paneWordByte(line, start-1) && !paneWordByte(line, end) {
				found = append(found, tok)
				break
			}
			from = start + 1
		}
	}
	return found
}

func paneWordByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i]
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func paneOrchestraAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == paneOrchestraImport {
			name := "orchestra"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			aliases[name] = true
		}
	}
	return aliases
}

func paneNameSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func paneGuardRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)
	return filepath.ToSlash(rel)
}
