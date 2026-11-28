package debt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// RV_ROOT points at a scratch clone. The index lives in that clone.
func TestRVPerf_Phases(t *testing.T) {
	root := os.Getenv("RV_ROOT")
	if root == "" {
		t.Skip("RV_ROOT unset")
	}
	ctx := context.Background()
	idx := filepath.Join(root, filepath.FromSlash(indexPath))
	if os.Getenv("RV_COLD") == "1" {
		_ = os.Remove(idx)
		for i := 1; ; i++ {
			start := time.Now()
			_, err := ScanWith(ctx, root, Options{MaxLines: 300, SizeOf: func(string) bool { return false }, Deadline: time.Now().Add(800 * time.Millisecond)})
			t.Logf("cold stop %d: %v err=%v", i, time.Since(start).Round(time.Millisecond), err)
			if err == nil || i > 60 {
				break
			}
		}
	}
	fi, _ := os.Stat(idx)
	if fi != nil {
		t.Logf("index size %d bytes", fi.Size())
	}
	changed := os.Getenv("RV_CHANGED")
	for i := 0; i < 3; i++ {
		t0 := time.Now()
		x := loadIndex(root)
		t1 := time.Now()
		files, err := x.refresh(ctx, root, time.Time{})
		t2 := time.Now()
		d := duplicates(files, x.Files)
		t3 := time.Now()
		x.save(root)
		t4 := time.Now()
		t.Logf("warm phases: load %v, refresh %v (%d product files), duplicates %v (%d items), save %v; err=%v",
			t1.Sub(t0).Round(time.Millisecond), t2.Sub(t1).Round(time.Millisecond), len(files), t3.Sub(t2).Round(time.Millisecond), len(d), t4.Sub(t3).Round(time.Millisecond), err)
		start := time.Now()
		items, err := ScanWith(ctx, root, Options{MaxLines: 300, SizeOf: func(f string) bool { return f == changed }, Deadline: time.Now().Add(800 * time.Millisecond)})
		t.Logf("stop-equivalent ScanWith: %v (%d items) err=%v", time.Since(start).Round(time.Millisecond), len(items), err)
	}
}
