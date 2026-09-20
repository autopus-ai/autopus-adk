package opencode

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func pluginDoc(t *testing.T, raw string) map[string]any {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	return d
}
func TestPluginConfigPreservesOptionsAndNativePrecedence(t *testing.T) {
	for _, tc := range []struct {
		raw, key string
		v2       bool
	}{
		{`{"plugin":[["user-plugin",{"nested":{"x":true}}]]}`, "plugin", false},
		{`{"plugin":[["user-plugin",{"nested":{"x":true}}]]}`, "plugin", true},
		{`{"plugins":[{"package":"user-plugin","options":{"nested":{"x":true}}}]}`, "plugins", true},
	} {
		d := pluginDoc(t, tc.raw)
		before := d[tc.key].([]any)[0]
		if err := mergePluginConfig(d, managedPluginPaths(nil), tc.v2, "/project"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d[tc.key].([]any)[0], before) {
			t.Fatal("options lost")
		}
		if !managedPluginRegistered(d, managedPluginPaths(nil)[0], tc.v2, "/project") {
			t.Fatal("managed path missing")
		}
	}
	d := pluginDoc(t, `{"plugin":["shadowed",".opencode/plugins/autopus-hooks.js"],"plugins":[{"package":"./.opencode/plugins/autopus-hooks.js","options":{"strict":true}},"effective"]}`)
	if err := mergePluginConfig(d, managedPluginPaths(nil), true, "/project"); err != nil {
		t.Fatal(err)
	}
	if len(d["plugins"].([]any)) != 2 || len(d["plugin"].([]any)) != 1 {
		t.Fatalf("duplicate or native precedence lost: %+v", d)
	}
}
func TestPluginConfigNewVersionShapeAndPathAliases(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		d := map[string]any{}
		if err := mergePluginConfig(d, managedPluginPaths(nil), v2, "/project"); err != nil {
			t.Fatal(err)
		}
		key := "plugin"
		want := ".opencode/plugins/autopus-hooks.js"
		if v2 {
			key = "plugins"
			want = "./" + want
		}
		if got := d[key].([]any)[0]; got != want {
			t.Fatalf("got %v want %s", got, want)
		}
	}
	d := pluginDoc(t, `{"plugins":["file:///project/.opencode/plugins/autopus-hooks.js"]}`)
	if err := mergePluginConfig(d, managedPluginPaths(nil), true, "/project"); err != nil {
		t.Fatal(err)
	}
	if len(d["plugins"].([]any)) != 1 {
		t.Fatal("file URL duplicated")
	}
}
func TestPluginConfigDisabledDoesNotRegisterOrReenable(t *testing.T) {
	d := pluginDoc(t, `{"plugins":["./.opencode/plugins/autopus-hooks.js","-*"]}`)
	if err := mergePluginConfig(d, managedPluginPaths(nil), true, "/project"); err != nil {
		t.Fatal(err)
	}
	if len(d["plugins"].([]any)) != 2 || managedPluginRegistered(d, managedPluginPaths(nil)[0], true, "/project") {
		t.Fatal("disabled plugin treated enabled")
	}
}

func TestPluginConfigRuntimePinnedRenderingAndValidation(t *testing.T) {
	for _, version := range []string{"1.18.7", "opencode v2.0.10"} {
		dir := t.TempDir()
		a := NewWithRoot(dir, WithCLIVersion(version))
		body, err := a.renderConfigDocument(nil)
		if err != nil {
			t.Fatal(err)
		}
		d := pluginDoc(t, body)
		key := "plugin"
		if strings.Contains(version, "v2") {
			key = "plugins"
		}
		if _, ok := d[key]; !ok {
			t.Fatalf("wrong config key for %s", version)
		}
		if err := os.WriteFile(filepath.Join(dir, configFile), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(markerBegin+"\n"+markerEnd), 0600); err != nil {
			t.Fatal(err)
		}
		findings, err := a.Validate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range findings {
			if strings.Contains(f.Message, "등록 누락") {
				t.Fatalf("valid config unregistered: %+v", f)
			}
		}
	}
}
func TestPluginConfigExplicitManagedDisableAndReenable(t *testing.T) {
	for _, control := range []string{"-autopus.hooks", "-autopus.*", "-*"} {
		d := map[string]any{"plugins": []any{control}}
		if err := mergePluginConfig(d, managedPluginPaths(nil), true, "/project"); err != nil {
			t.Fatal(err)
		}
		if len(d["plugins"].([]any)) != 1 || managedPluginRegistered(d, managedPluginPaths(nil)[0], true, "/project") {
			t.Fatalf("override explicit disable %s", control)
		}
	}
	d := pluginDoc(t, `{"plugins":["./.opencode/plugins/autopus-hooks.js","-autopus.hooks","autopus.hooks"]}`)
	if !managedPluginRegistered(d, managedPluginPaths(nil)[0], true, "/project") {
		t.Fatal("ordered reenable ignored")
	}
}

func TestPluginConfigRejectsFutureRuntimeBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	a := NewWithRoot(dir, WithCLIVersion("opencode v3.0.0"))
	if err := a.InjectOrchestraPlugin("./extra.js"); err == nil {
		t.Fatal("unsupported future runtime accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, configFile)); !os.IsNotExist(err) {
		t.Fatal("future runtime wrote config")
	}
}

func TestPluginConfigRelativeRootRecognizesAbsoluteManagedPaths(t *testing.T) {
	for _, root := range []string{".", "relative-project"} {
		target := managedPluginPaths(nil)[0]
		absolute, err := filepath.Abs(filepath.Join(root, target))
		if err != nil {
			t.Fatal(err)
		}
		fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
		for _, entry := range []string{absolute, fileURL} {
			d := map[string]any{"plugins": []any{entry}}
			if !managedPluginRegistered(d, target, true, root) {
				t.Fatalf("root %q fails registration of %q", root, entry)
			}
			if err := mergePluginConfig(d, []string{target}, true, root); err != nil {
				t.Fatal(err)
			}
			if len(d["plugins"].([]any)) != 1 {
				t.Fatalf("root %q duplicated %q", root, entry)
			}
		}
	}
}
