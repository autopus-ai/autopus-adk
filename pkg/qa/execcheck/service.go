package execcheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// lockedBuffer collects a running server's output while the requests read it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runService starts argv as a server on a free local port, waits until it is
// ready, sends the requests in order, and stops it. A server that exits
// before it is ready is a product verdict, like a build that names a product
// file: the user would see it fail to start. A server that never listens on
// the port is not: the case may not have passed $PORT.
func runService(ctx context.Context, timeout time.Duration, work string, argv []string, x testscenario.Exec) (Result, error) {
	port, err := freePort()
	if err != nil {
		return Result{}, err
	}
	portStr := strconv.Itoa(port)
	for i := range argv {
		argv[i] = strings.ReplaceAll(argv[i], testscenario.PlaceholderPort, portStr)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "PORT="+portStr)
	var output lockedBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	killGroupOnCancel(cmd)
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", argv[0], err)
	}
	// exited is closed once the server is gone and waitErr is set, so both
	// the readiness wait and the teardown can observe it.
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	defer func() {
		cancel()
		<-exited
	}()

	base := "http://127.0.0.1:" + portStr
	client := &http.Client{Timeout: 10 * time.Second}
	if err := waitReady(ctx, client, base, x.Ready, exited, func() error { return waitErr }); err != nil {
		if exitErr, ok := err.(serverExited); ok {
			return Result{ExitCode: exitErr.code, Stderr: output.String(), Mismatches: []Mismatch{
				{"service", "the server starts and serves on $PORT", tail(output.String())},
			}}, nil
		}
		return Result{}, fmt.Errorf("%s: %w (pass $PORT in argv or read the PORT environment variable)", argv[0], err)
	}
	res := Result{Mismatches: []Mismatch{}}
	for i, r := range x.Requests {
		status, header, body, err := send(ctx, client, base, r)
		label := fmt.Sprintf("request %d %s %s", i+1, strings.ToUpper(r.Method), r.Path)
		if err != nil {
			res.Mismatches = append(res.Mismatches, Mismatch{label, "a response", err.Error()})
			break
		}
		res.Stdout += fmt.Sprintf("%s -> %d\n%s\n", label, status, body)
		res.Mismatches = append(res.Mismatches, compareResponse(label, r.Expect, status, header, body)...)
	}
	res.Stderr = output.String()
	return res, nil
}

type serverExited struct{ code int }

func (e serverExited) Error() string {
	return fmt.Sprintf("the server exited with %d before it was ready", e.code)
}

func waitReady(ctx context.Context, client *http.Client, base, ready string, exited <-chan struct{}, waitErr func() error) error {
	host := strings.TrimPrefix(base, "http://")
	for {
		select {
		case <-exited:
			err := waitErr()
			code := -1
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else if err == nil {
				code = 0
			}
			return serverExited{code: code}
		case <-ctx.Done():
			return fmt.Errorf("the server did not become ready on %s", host)
		case <-time.After(100 * time.Millisecond):
		}
		if ready == "" {
			if conn, err := net.DialTimeout("tcp", host, time.Second); err == nil {
				_ = conn.Close()
				return nil
			}
			continue
		}
		if resp, err := client.Get(base + ready); err == nil {
			_ = resp.Body.Close()
			return nil
		}
	}
}

func send(ctx context.Context, client *http.Client, base string, r testscenario.HTTPRequest) (int, http.Header, string, error) {
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(r.Method), base+r.Path, strings.NewReader(r.Body))
	if err != nil {
		return 0, nil, "", err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, resp.Header, string(body), err
}

func compareResponse(label string, want testscenario.HTTPExpect, status int, header http.Header, body string) []Mismatch {
	var out []Mismatch
	if want.Status != nil && *want.Status != status {
		out = append(out, Mismatch{label + " status", strconv.Itoa(*want.Status), strconv.Itoa(status)})
	}
	if want.Body != nil && *want.Body != body {
		out = append(out, Mismatch{label + " body", *want.Body, body})
	}
	for _, s := range want.BodyContains {
		if !strings.Contains(body, s) {
			out = append(out, Mismatch{label + " body_contains", s, body})
		}
	}
	for _, s := range want.BodyNotContains {
		if strings.Contains(body, s) {
			out = append(out, Mismatch{label + " body_not_contains", "no " + strconv.Quote(s), body})
		}
	}
	for name, v := range want.Headers {
		if got := header.Get(name); !strings.Contains(got, v) {
			out = append(out, Mismatch{label + " header " + name, v, got})
		}
	}
	return out
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
