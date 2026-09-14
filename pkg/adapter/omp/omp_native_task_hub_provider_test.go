package omp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type ompNativeChatRequest struct {
	Model    string            `json:"model"`
	Stream   bool              `json:"stream"`
	Tools    []json.RawMessage `json:"tools"`
	Messages []json.RawMessage `json:"messages"`
}

type ompNativeProviderReceipt struct {
	Requests            int
	ParentStages        int
	ChildRequests       map[string]int
	AuthHeaders         int
	CredentialLeaks     int
	UnexpectedEndpoints int
	Failure             string
}

type ompNativeProvider struct {
	server *httptest.Server

	mu                                sync.Mutex
	parentStage                       int
	childRequests                     map[string]int
	requestCount                      int
	authHeaders                       int
	credentialLeaks                   int
	unexpectedEndpoints               int
	failure                           string
	childrenReady                     chan struct{}
	childrenReadyOnce                 sync.Once
	alphaRelease, betaRelease         chan struct{}
	alphaReleaseOnce, betaReleaseOnce sync.Once
}

func newOMPNativeProvider(t *testing.T) *ompNativeProvider {
	t.Helper()
	provider := &ompNativeProvider{
		childRequests: map[string]int{ompNativeAlphaID: 0, ompNativeBetaID: 0},
		childrenReady: make(chan struct{}),
		alphaRelease:  make(chan struct{}),
		betaRelease:   make(chan struct{}),
	}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.handle))
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *ompNativeProvider) handle(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.requestCount++
	p.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		p.mu.Lock()
		p.unexpectedEndpoints++
		p.mu.Unlock()
		p.reject(w, "unexpected_endpoint")
		return
	}
	if ompNativeRequestHasAuth(r.Header) {
		p.mu.Lock()
		p.authHeaders++
		p.mu.Unlock()
		p.reject(w, "unexpected_auth_header")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		p.reject(w, "request_oversized")
		return
	}
	if strings.Contains(string(body), ompNativeCredential) {
		p.mu.Lock()
		p.credentialLeaks++
		p.mu.Unlock()
		p.reject(w, "credential_leak")
		return
	}
	var request ompNativeChatRequest
	if json.Unmarshal(body, &request) != nil || request.Model != ompLiveModel || !request.Stream {
		p.reject(w, "invalid_request_shape")
		return
	}
	// The parent conversation is the one carrying the smoke prompt; a child
	// only ever receives the shared context plus its own assignment. Tool
	// shape is no longer a discriminator: the bundled `reviewer` declares
	// `spawns: [scout]`, so its request carries a `task` tool too, and
	// "has task tool ⇒ parent" misrouted that child into the parent staging
	// sequence. That rule held only while the project shipped its own
	// non-spawning agent definitions.
	if strings.Contains(string(body), ompNativeSmokeToken) {
		p.handleParent(w, r, request, body)
		return
	}
	p.handleChild(w, r, request, body)
}

func (p *ompNativeProvider) handleParent(
	w http.ResponseWriter,
	r *http.Request,
	request ompNativeChatRequest,
	body []byte,
) {
	p.mu.Lock()
	stage := p.parentStage
	p.mu.Unlock()
	lastTool := ompNativeLastToolContent(request.Messages)
	var validationErr error
	switch stage {
	case 0:
		if !strings.Contains(string(body), ompNativeSmokeToken) {
			validationErr = fmt.Errorf("smoke prompt missing")
		} else {
			validationErr = validateOMPNativeParentTools(request.Tools)
		}
	case 1:
		validationErr = ompNativeRequireTokens(lastTool,
			"Spawned 2 background agents", ompNativeAlphaID, ompNativeBetaID)
		if validationErr == nil {
			select {
			case <-p.childrenReady:
			case <-r.Context().Done():
				validationErr = fmt.Errorf("children did not reach provider")
			case <-time.After(8 * time.Second):
				validationErr = fmt.Errorf("children did not start")
			}
		}
	case 2:
		validationErr = ompNativeRequireTokens(lastTool,
			ompNativeAlphaID, ompNativeBetaID, "running")
	case 3:
		validationErr = ompNativeRequireTokens(lastTool,
			"Still Running (2)", ompNativeAlphaID, ompNativeBetaID)
	case 4:
		validationErr = ompNativeRequireTokens(lastTool,
			"Completed (1)", ompNativeAlphaID,
			"owned_paths", "changed_files", "verification", "blockers", "next_required_step",
			"native-alpha-read-only")
	case 5:
		validationErr = ompNativeRequireTokens(lastTool,
			"Still Running (1)", ompNativeBetaID)
	case 6:
		validationErr = ompNativeRequireTokens(lastTool,
			"Completed (1)", ompNativeBetaID,
			"owned_paths", "changed_files", "verification", "blockers", "next_required_step",
			"native-beta-read-only")
	case 7:
		validationErr = ompNativeRequireTokens(lastTool,
			ompNativeAlphaID, ompNativeBetaID, "idle")
	default:
		validationErr = fmt.Errorf("parent request budget exceeded")
	}
	if validationErr != nil {
		p.reject(w, fmt.Sprintf("parent_stage_%d:%v", stage, validationErr))
		return
	}
	p.mu.Lock()
	if p.parentStage != stage {
		p.mu.Unlock()
		p.reject(w, "parent_stage_race")
		return
	}
	p.parentStage++
	p.mu.Unlock()

	switch stage {
	case 0:
		writeOMPNativeToolSSE(w, 1, "task", ompNativeTaskArguments())
	case 1:
		writeOMPNativeToolSSE(w, 2, "hub", map[string]any{
			"i": "Listing child identities", "op": "list",
		})
	case 2:
		writeOMPNativeToolSSE(w, 3, "hub", map[string]any{
			"i": "Inspecting child jobs", "op": "jobs",
		})
	case 3:
		writeOMPNativeToolSSE(w, 4, "hub", map[string]any{
			"i": "Awaiting alpha receipt", "op": "wait",
			"ids": []string{ompNativeAlphaID}, "timeoutMs": 15_000,
		})
	case 4:
		writeOMPNativeToolSSE(w, 5, "hub", map[string]any{
			"i": "Inspecting remaining child", "op": "jobs",
		})
	case 5:
		writeOMPNativeToolSSE(w, 6, "hub", map[string]any{
			"i": "Awaiting beta receipt", "op": "wait",
			"ids": []string{ompNativeBetaID}, "timeoutMs": 15_000,
		})
	case 6:
		writeOMPNativeToolSSE(w, 7, "hub", map[string]any{
			"i": "Confirming child lifecycle", "op": "list",
		})
	case 7:
		writeOMPNativeFinalSSE(w, 8, "native task hub lifecycle complete")
	}
}

func (p *ompNativeProvider) handleChild(
	w http.ResponseWriter,
	r *http.Request,
	request ompNativeChatRequest,
	body []byte,
) {
	bodyText := string(body)
	id := ""
	if strings.Contains(bodyText, "NATIVE_CHILD_ALPHA") {
		id = ompNativeAlphaID
	}
	if strings.Contains(bodyText, "NATIVE_CHILD_BETA") {
		if id != "" {
			p.reject(w, "child_assignment_ambiguous")
			return
		}
		id = ompNativeBetaID
	}
	if id == "" {
		p.reject(w, "unknown_child_assignment")
		return
	}
	if err := validateOMPNativeChildTools(id, request.Tools); err != nil {
		p.reject(w, "child_schema:"+err.Error())
		return
	}
	p.mu.Lock()
	p.childRequests[id]++
	count := p.childRequests[id]
	ready := p.childRequests[ompNativeAlphaID] == 1 && p.childRequests[ompNativeBetaID] == 1
	p.mu.Unlock()
	if count != 1 {
		p.reject(w, "child_request_budget_exceeded:"+id)
		return
	}
	if ready {
		p.childrenReadyOnce.Do(func() { close(p.childrenReady) })
	}
	release := p.alphaRelease
	if id == ompNativeBetaID {
		release = p.betaRelease
	}
	select {
	case <-release:
	case <-r.Context().Done():
		p.reject(w, "child_request_cancelled:"+id)
		return
	case <-time.After(20 * time.Second):
		p.reject(w, "child_release_timeout:"+id)
		return
	}
	writeOMPNativeToolSSE(w, 100+count, "yield", map[string]any{
		"i":      "Submitting read-only receipt",
		"type":   nil,
		"result": map[string]any{"data": ompNativeExpectedReceipt(id)},
	})
}

func (p *ompNativeProvider) release(id string) {
	if id == ompNativeAlphaID {
		p.alphaReleaseOnce.Do(func() { close(p.alphaRelease) })
		return
	}
	p.betaReleaseOnce.Do(func() { close(p.betaRelease) })
}

func (p *ompNativeProvider) releaseAll() {
	p.release(ompNativeAlphaID)
	p.release(ompNativeBetaID)
}

func (p *ompNativeProvider) receipt() ompNativeProviderReceipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	children := make(map[string]int, len(p.childRequests))
	for id, count := range p.childRequests {
		children[id] = count
	}
	return ompNativeProviderReceipt{
		Requests: p.requestCount, ParentStages: p.parentStage, ChildRequests: children,
		AuthHeaders: p.authHeaders, CredentialLeaks: p.credentialLeaks,
		UnexpectedEndpoints: p.unexpectedEndpoints, Failure: p.failure,
	}
}

func (p *ompNativeProvider) reject(w http.ResponseWriter, reason string) {
	p.mu.Lock()
	if p.failure == "" {
		p.failure = reason
	}
	p.mu.Unlock()
	p.releaseAll()
	http.Error(w, "native fixture rejected request", http.StatusBadRequest)
}
