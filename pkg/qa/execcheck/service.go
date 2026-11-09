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
	base := "http://127.0.0.1:" + portStr
	client := &http.Client{Timeout: 10 * time.Second}
	var output lockedBuffer
	srv, res, err := startServer(ctx, client, work, argv, portStr, base, x.Ready, &output)
	if srv == nil {
		return res, err
	}
	defer func() { srv.kill() }()

	res = Result{Mismatches: []Mismatch{}}
	for i, r := range x.Requests {
		if r.Restart {
			// The same server comes back on the same port, so a request after
			// it reads what the product kept across a restart.
			srv.stop()
			res.Stdout += fmt.Sprintf("step %d: restarted the server\n", i+1)
			next, failed, err := startServer(ctx, client, work, argv, portStr, base, x.Ready, &output)
			if next == nil {
				if err != nil {
					return Result{}, err
				}
				failed.Mismatches[0].Field = fmt.Sprintf("step %d restart", i+1)
				failed.Stdout = res.Stdout
				return failed, nil
			}
			srv = next
			continue
		}
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

// server is one run of the product's server process.
type server struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	exited chan struct{}
}

// startServer starts argv and waits until it is ready. A server that exits
// before it is ready returns a product verdict as the Result with a nil
// server; one that never listens returns an error.
func startServer(parent context.Context, client *http.Client, work string, argv []string, port, base, ready string, output *lockedBuffer) (*server, Result, error) {
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "PORT="+port)
	cmd.Stdout, cmd.Stderr = output, output
	killGroupOnCancel(cmd)
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, Result{}, fmt.Errorf("start %s: %w", argv[0], err)
	}
	// exited is closed once the server is gone and waitErr is set, so both
	// the readiness wait and the teardown can observe it.
	srv := &server{cmd: cmd, cancel: cancel, exited: make(chan struct{})}
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(srv.exited)
	}()
	if err := waitReady(ctx, client, base, ready, srv.exited, func() error { return waitErr }); err != nil {
		srv.kill()
		if exitErr, ok := err.(serverExited); ok {
			return nil, Result{ExitCode: exitErr.code, Stderr: output.String(), Mismatches: []Mismatch{
				{"service", "the server starts and serves on $PORT", tail(output.String())},
			}}, nil
		}
		return nil, Result{}, fmt.Errorf("%s: %w (pass $PORT in argv or read the PORT environment variable)", argv[0], err)
	}
	return srv, Result{}, nil
}

// stop asks the server to shut down the way a person or a supervisor would,
// and kills its process group if it has not exited within five seconds.
func (s *server) stop() {
	terminateGroup(s.cmd)
	select {
	case <-s.exited:
	case <-time.After(5 * time.Second):
	}
	s.kill()
}

func (s *server) kill() {
	s.cancel()
	<-s.exited
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
