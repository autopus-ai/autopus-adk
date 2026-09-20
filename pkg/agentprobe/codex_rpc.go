package agentprobe

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

type codexRPCMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}
type codexRPC interface {
	call(context.Context, string, any) (json.RawMessage, error)
	notify(string, any) error
	next(context.Context) (codexRPCMessage, error)
	buffered() []codexRPCMessage
}
type codexRPCClient struct {
	input    io.WriteCloser
	output   io.ReadCloser
	messages chan codexRPCMessage
	pending  []codexRPCMessage
	sequence int
	done     chan struct{}
	err      error
	mu       sync.Mutex
	stop     context.CancelFunc
	cmd      *exec.Cmd
	wait     chan error
}

func startCodexRPC(ctx context.Context, executable, cwd string) (*codexRPCClient, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, executable, "app-server", "--listen", "stdio://")
	cmd.Dir = cwd
	cmd.WaitDelay = 2 * time.Second
	configureCodexProbeProcess(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = input.Close()
		return nil, err
	}
	client := &codexRPCClient{input: input, output: output, messages: make(chan codexRPCMessage, 64), done: make(chan struct{}), stop: cancel, cmd: cmd, wait: make(chan error, 1)}
	cmd.Stderr = &codexDiscardLimit{remaining: MaxEvidenceBytes, stop: func() { client.setError(codexDiagnostic("transport_stderr_limit", nil)); cancel() }}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = input.Close()
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	go func() {
		defer close(client.done)
		scanner := bufio.NewScanner(io.LimitReader(output, MaxEvidenceBytes+1))
		scanner.Buffer(make([]byte, 4096), 256*1024)
		total := 0
		for scanner.Scan() {
			total += len(scanner.Bytes()) + 1
			if total > MaxEvidenceBytes {
				client.setError(codexDiagnostic("transport_output_limit", nil))
				cancel()
				return
			}
			var message codexRPCMessage
			if json.Unmarshal(scanner.Bytes(), &message) != nil {
				client.setError(codexDiagnostic("transport_frame_invalid", nil))
				cancel()
				return
			}
			retain, err := codexRetainMessage(message)
			if err != nil {
				client.setError(codexDiagnostic("transport_lifecycle_frame_invalid", err))
				cancel()
				return
			}
			if !retain {
				continue
			}
			select {
			case client.messages <- message:
			case <-ctx.Done():
				client.setError(ctx.Err())
				return
			}
		}
		if err := scanner.Err(); err != nil {
			client.setError(codexDiagnostic("transport_stream_read_failed", nil))
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = output.Close()
		case <-client.done:
		}
	}()
	go func() { <-client.done; client.wait <- cmd.Wait() }()
	return client, nil
}
func (c *codexRPCClient) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = err
	}
}
func (c *codexRPCClient) close() { c.stop(); _ = c.input.Close(); <-c.wait }
func (c *codexRPCClient) write(value any) error {
	if err := json.NewEncoder(c.input).Encode(value); err != nil {
		return codexDiagnostic("transport_write_failed", err)
	}
	return nil
}
func (c *codexRPCClient) notify(method string, params any) error {
	return c.write(map[string]any{"method": method, "params": params})
}
func (c *codexRPCClient) receive(ctx context.Context) (codexRPCMessage, error) {
	select {
	case m := <-c.messages:
		return m, nil
	case <-c.done:
		select {
		case m := <-c.messages:
			return m, nil
		default:
		}
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = io.EOF
		}
		return codexRPCMessage{}, err
	case <-ctx.Done():
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err != nil {
			return codexRPCMessage{}, err
		}
		return codexRPCMessage{}, ctx.Err()
	}
}
func (c *codexRPCClient) next(ctx context.Context) (codexRPCMessage, error) {
	for {
		var m codexRPCMessage
		if len(c.pending) > 0 {
			m = c.pending[0]
			c.pending = c.pending[1:]
		} else {
			var err error
			m, err = c.receive(ctx)
			if err != nil {
				return m, err
			}
		}
		if len(m.ID) > 0 && m.Method != "" {
			if err := c.write(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "probe does not support client requests"}}); err != nil {
				return codexRPCMessage{}, err
			}
			continue
		}
		retain, err := codexRetainMessage(m)
		if err != nil {
			return codexRPCMessage{}, err
		}
		if !retain {
			continue
		}
		return m, nil
	}
}
func (c *codexRPCClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.sequence++
	id := c.sequence
	if err := c.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		m, err := c.receive(ctx)
		if err != nil {
			return nil, err
		}
		if len(m.ID) > 0 && m.Method != "" {
			// The probe never approves server-requested commands or edits.
			if err := c.write(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "probe does not support client requests"}}); err != nil {
				return nil, err
			}
			continue
		}
		var received int
		if len(m.ID) > 0 && json.Unmarshal(m.ID, &received) == nil && received == id {
			if len(m.Error) > 0 && string(m.Error) != "null" {
				return nil, codexDiagnostic(codexRPCFailureCode(method), nil)
			}
			return m.Result, nil
		}
		if m.Method != "" {
			retain, err := codexRetainMessage(m)
			if err != nil {
				return nil, err
			}
			if !retain {
				continue
			}
			if len(c.pending) >= 256 {
				return nil, codexDiagnostic("transport_lifecycle_queue_limit", nil)
			}
			c.pending = append(c.pending, m)
		}
	}
}

type codexDiscardLimit struct {
	remaining int
	stop      context.CancelFunc
}

func (w *codexDiscardLimit) Write(p []byte) (int, error) {
	w.remaining -= len(p)
	if w.remaining < 0 {
		w.stop()
		return 0, codexDiagnostic("transport_stderr_limit", nil)
	}
	return len(p), nil
}

func (c *codexRPCClient) buffered() []codexRPCMessage {
	result := c.pending
	c.pending = nil
	return result
}
