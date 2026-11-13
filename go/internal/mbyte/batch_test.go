package mbyte

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeDir creates dir/<name> files with the given contents and returns the dir.
func makeDir(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, data := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

// TestBatchInsertTable: insert --preset png over a dir writes one output per
// input and reports them all ok.
func TestBatchInsertTable(t *testing.T) {
	in := makeDir(t, map[string][]byte{
		"a.bin": []byte("alpha"),
		"b.bin": []byte("bravo"),
	})
	out := t.TempDir()

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png"}
	if err := RunBatch(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	got := outBuf.String()
	if !strings.Contains(got, "2 file(s): 2 ok, 0 failed") {
		t.Fatalf("summary wrong:\n%s", got)
	}
	for _, name := range []string{"a.bin.png", "b.bin.png"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("expected output %s: %v", name, err)
		}
	}
}

// TestBatchJSON: --json emits a parseable array with the expected fields.
func TestBatchJSON(t *testing.T) {
	in := makeDir(t, map[string][]byte{"a.bin": []byte("alpha")})
	out := t.TempDir()

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png", JSON: true}
	if err := RunBatch(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	var results []BatchResult
	if err := json.Unmarshal([]byte(outBuf.String()), &results); err != nil {
		t.Fatalf("JSON parse: %v\n%s", err, outBuf.String())
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].DetectedFormat != "PNG image data" || results[0].PayloadStatus != "MATCH" {
		t.Fatalf("unexpected fields: %+v", results[0])
	}
}

// TestBatchRecursive: -r processes files in subdirectories; without it, only
// the top level.
func TestBatchRecursive(t *testing.T) {
	in := makeDir(t, map[string][]byte{
		"top.bin":      []byte("t"),
		"sub/deep.bin": []byte("d"),
	})

	// non-recursive: only top.bin
	files, err := collectFiles(in, false)
	if err != nil {
		t.Fatalf("collectFiles: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "top.bin" {
		t.Fatalf("non-recursive should see only top.bin, got %v", files)
	}
	// recursive: both
	files, err = collectFiles(in, true)
	if err != nil {
		t.Fatalf("collectFiles rec: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("recursive should see 2 files, got %v", files)
	}
}

// TestBatchPartialFailure: a file that cannot be processed is reported as a
// failure and RunBatch returns ErrBatchFailures (exit 1), while good files
// still succeed.
func TestBatchPartialFailure(t *testing.T) {
	in := makeDir(t, map[string][]byte{
		"good.png": append(decodeHex(t, "89504e470d0a1a0a"), []byte("REST")...), // recognized -> replace ok
		"bad.bin":  []byte("not a known signature"),                             // unrecognized -> replace fails
	})
	out := t.TempDir()

	var outBuf, errBuf strings.Builder
	// replace (non-raw) needs a recognized current signature; bad.bin has none.
	opts := RunOptions{Command: "replace", InputFile: in, Output: out, Preset: "gif89a"}
	err := RunBatch(opts, IO{Out: &outBuf, Err: &errBuf})
	if !errors.Is(err, ErrBatchFailures) {
		t.Fatalf("expected ErrBatchFailures, got %v", err)
	}
	got := outBuf.String()
	if !strings.Contains(got, "1 ok, 1 failed") {
		t.Fatalf("summary wrong:\n%s", got)
	}
	if !strings.Contains(got, "FAIL") || !strings.Contains(got, "error:") {
		t.Fatalf("failure row missing:\n%s", got)
	}
	// good file was still written
	if _, err := os.Stat(filepath.Join(out, "good.png.gif")); err != nil {
		t.Errorf("good file should be written: %v", err)
	}
}

// TestBatchDryRunNoWrite: --dry-run computes results but writes nothing.
func TestBatchDryRunNoWrite(t *testing.T) {
	in := makeDir(t, map[string][]byte{"a.bin": []byte("alpha")})
	out := t.TempDir()

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png", DryRun: true}
	if err := RunBatch(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("RunBatch: %v", err)
	}
	if !strings.Contains(outBuf.String(), "would process (dry-run)") {
		t.Fatalf("missing dry-run note:\n%s", outBuf.String())
	}
	if _, err := os.Stat(filepath.Join(out, "a.bin.png")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write outputs")
	}
}

// TestDispatchRoutesDirToBatch: Dispatch sends a directory to batch and a file
// to the single-file path.
func TestDispatchRoutesDirToBatch(t *testing.T) {
	in := makeDir(t, map[string][]byte{"a.bin": []byte("alpha")})
	out := t.TempDir()
	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png"}
	if err := Dispatch(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("Dispatch(dir): %v", err)
	}
	if !strings.Contains(outBuf.String(), "[batch]") {
		t.Fatalf("directory should route to batch:\n%s", outBuf.String())
	}
}
