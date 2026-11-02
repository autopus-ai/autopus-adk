package worker

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/insajin/autopus-adk/pkg/worker/a2a"
	"github.com/insajin/autopus-adk/pkg/worker/adapter"
	"github.com/insajin/autopus-adk/pkg/worker/audit"
	"github.com/insajin/autopus-adk/pkg/worker/auth"
	"github.com/insajin/autopus-adk/pkg/worker/knowledge"
	workerNet "github.com/insajin/autopus-adk/pkg/worker/net"
	"github.com/insajin/autopus-adk/pkg/worker/parallel"
	"github.com/insajin/autopus-adk/pkg/worker/pidlock"
	"github.com/insajin/autopus-adk/pkg/worker/reaper"
	"github.com/insajin/autopus-adk/pkg/worker/routing"
	"github.com/insajin/autopus-adk/pkg/worker/setup"
)

// @AX:ANCHOR: [AUTO] worker runtime configuration boundary assembled by host resolution and consumed by WorkerLoop startup.
// @AX:REASON: Worktree isolation, fallback override, audit, auth, and provider fields coordinate desktop worker safety behavior.
// LoopConfig holds configuration for the WorkerLoop.
type LoopConfig struct {
	BackendURL        string
	WorkerName        string
	MemoryAgentID     string
	Skills            []string
	Providers         []string
	Provider          adapter.ProviderAdapter
	MCPConfig         string                // path to worker-mcp.json
	WorkDir           string                // working directory for subprocesses
	AuthToken         string                // bearer token for backend auth
	Router            *routing.Router       // optional model router (nil = no routing)
	CredentialStore   setup.CredentialStore // Secure credential storage (Keychain/encrypted file); nil skips JWT token refresh.
	AuditLogPath      string                // audit log file path (default: {WorkDir}/.autopus/audit.jsonl)
	AuditMaxSize      int64                 // max log size before rotation (default: 10MB)
	AuditMaxAge       time.Duration         // max age of rotated files (default: 7 days)
	WorkspaceID       string                // workspace identifier for registration and knowledge/memory search
	MaxConcurrency    int                   // max parallel tasks (0 = default slot cap, 1 = sequential)
	WorktreeIsolation bool                  // enable worktree isolation for parallel tasks
	// WorktreeFallbackOverrideReason permits explicit root-worktree fallback when isolation is unavailable.
	WorktreeFallbackOverrideReason string
	KnowledgeSync                  bool   // enable local knowledge context loading
	KnowledgeDir                   string // local knowledge directory hint (defaults to WorkDir)
	// RecordAgentRun is the single injected persistence boundary for final task usage.
	RecordAgentRun func(telemetry.AgentRun) error
}

// WorkerLoop integrates A2A Server, ProviderAdapter, ContextBuilder, and StreamParser.
// It receives tasks via A2A, builds prompts, spawns CLI subprocesses, and reports results.
type WorkerLoop struct {
	config            LoopConfig
	server            *a2a.Server
	builder           ContextBuilder
	tuiProgram        *tea.Program
	approvalMu        sync.Mutex
	pendingApprovals  map[string]a2a.ApprovalRequestParams
	observerMu        sync.RWMutex
	hostObservers     []HostObserver
	authRefresher     *auth.TokenRefresher
	authReconnector   *auth.Reconnector
	netMonitor        *workerNet.NetMonitor
	lifecycleCtx      context.Context
	lifecycleCancel   context.CancelFunc
	auditWriter       *audit.RotatingWriter
	knowledgeSearcher *knowledge.KnowledgeSearcher
	memorySearcher    *knowledge.MemorySearcher
	semaphore         *parallel.TaskSemaphore
	worktreeManager   *parallel.WorktreeManager
	auditLogger       *slogAuditLogger
	pidLock           *pidlock.Lock
	zombieReaper      *reaper.Reaper
	postconditionGit  postconditionGit // zero value: the worker's own git environment
}

// NewWorkerLoop creates a WorkerLoop with the given configuration.
func NewWorkerLoop(config LoopConfig) *WorkerLoop {
	wl := &WorkerLoop{
		config:           config,
		pendingApprovals: make(map[string]a2a.ApprovalRequestParams),
		auditLogger:      newSlogAuditLogger(3),
	}

	serverCfg := a2a.ServerConfig{
		BackendURL:            config.BackendURL,
		WorkerName:            config.WorkerName,
		WorkspaceID:           config.WorkspaceID,
		Skills:                config.Skills,
		Providers:             config.Providers,
		Handler:               wl.handleTask,
		AuthToken:             config.AuthToken,
		ApprovalCallback:      wl.handleApproval,
		DispatchIssueCallback: wl.handleDispatchIssue,
		OnConnectionExhausted: wl.activateFallbackPoller,
	}
	wl.server = a2a.NewServer(serverCfg)

	return wl
}
