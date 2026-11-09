package main_test

// Hidden oracle for the itemsapi fixture. The agent never sees it. It pins
// only what the request states: names come back as sent, in order, joined
// by commas; POST answers 201; another method answers 405.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "itemsapi")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", strings.TrimPrefix(base, "http://")); err == nil {
			_ = c.Close()
			return base
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("server did not listen on PORT")
	return ""
}

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

// Names come back exactly as sent (the planted bug upper-cases them).
func TestOracle_NamesComeBackAsSentInOrder(t *testing.T) {
	base := server(t)
	for _, name := range []string{"apple", "Banana"} {
		if code, _ := do(t, http.MethodPost, base+"/items", name); code != http.StatusCreated {
			t.Fatalf("POST %s: want 201, got %d", name, code)
		}
	}
	if code, body := do(t, http.MethodGet, base+"/items", ""); code != http.StatusOK || strings.TrimSpace(body) != "apple,Banana" {
		t.Fatalf("GET /items: want 200 apple,Banana, got %d %q", code, body)
	}
}

func TestOracle_OtherMethodsAre405(t *testing.T) {
	if code, _ := do(t, http.MethodPut, server(t)+"/items", "x"); code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: want 405, got %d", code)
	}
}
