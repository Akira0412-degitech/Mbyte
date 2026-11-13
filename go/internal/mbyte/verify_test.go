package mbyte

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realPNG returns the bytes of a valid 1x1 PNG.
func realPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// TestCheckBytesRealPNGPasses: a genuine PNG decodes -> PASS.
func TestCheckBytesRealPNGPasses(t *testing.T) {
	status, format, detail := CheckBytes(realPNG(t))
	if status != StructPass {
		t.Fatalf("real PNG: status=%v detail=%q, want PASS", status, detail)
	}
	if format != "PNG image data" {
		t.Fatalf("format=%q", format)
	}
}

// TestCheckBytesFakePNGFails: PNG signature + junk matches the signature but
// fails to decode -> FAIL. This is the core Step 6 demonstration.
func TestCheckBytesFakePNGFails(t *testing.T) {
	fake := append(decodeHex(t, "89504e470d0a1a0a"), []byte("not a real PNG body")...)
	status, format, detail := CheckBytes(fake)
	if status != StructFail {
		t.Fatalf("fake PNG: status=%v, want FAIL", status)
	}
	if format != "PNG image data" {
		t.Fatalf("signature should still match PNG, got %q", format)
	}
	if detail == "" {
		t.Fatal("expected a verbatim decoder error detail")
	}
}

// TestCheckBytesFakeZipFails: ZIP needs a valid End-of-Central-Directory at the
// end, so a prepended signature alone never decodes.
func TestCheckBytesFakeZipFails(t *testing.T) {
	fake := append(decodeHex(t, "504b0304"), []byte("payload")...)
	status, _, _ := CheckBytes(fake)
	if status != StructFail {
		t.Fatalf("fake zip: status=%v, want FAIL", status)
	}
}

// TestCheckBytesUnknownSkips: an unrecognized format has no checker -> SKIP.
func TestCheckBytesUnknownSkips(t *testing.T) {
	status, _, _ := CheckBytes([]byte("just some text, no signature"))
	if status != StructSkip {
		t.Fatalf("unknown: status=%v, want SKIP", status)
	}
}

// TestRunCheckFailExit: RunCheck on a fake PNG prints the report and returns
// ErrCheckFailed.
func TestRunCheckFailExit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "shell.png")
	fake := append(decodeHex(t, "89504e470d0a1a0a"), []byte("x")...)
	if err := os.WriteFile(p, fake, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out, errb strings.Builder
	err := RunCheck(p, IO{Out: &out, Err: &errb})
	if !errors.Is(err, ErrCheckFailed) {
		t.Fatalf("want ErrCheckFailed, got %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "[signature] PNG image data") {
		t.Fatalf("missing signature line:\n%s", got)
	}
	if !strings.Contains(got, "[structure] FAIL (") {
		t.Fatalf("missing structure FAIL line:\n%s", got)
	}
}

// TestRunCheckPass: RunCheck on a real PNG returns nil and reports PASS.
func TestRunCheckPass(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "real.png")
	if err := os.WriteFile(p, realPNG(t), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var out, errb strings.Builder
	if err := RunCheck(p, IO{Out: &out, Err: &errb}); err != nil {
		t.Fatalf("RunCheck: %v", err)
	}
	if !strings.Contains(out.String(), "[structure] PASS") {
		t.Fatalf("expected PASS:\n%s", out.String())
	}
}

// TestRunAutoCheckAppended: insert emits a [check] line on stderr by default,
// and omits it with --no-check.
func TestRunAutoCheckAppended(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "p.bin")
	if err := os.WriteFile(in, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := filepath.Join(dir, "p.png")

	// default: auto-check present (fake PNG -> FAIL)
	var o1, e1 strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png"}
	if err := Run(opts, IO{Out: &o1, Err: &e1}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(e1.String(), "[check] structure: FAIL") {
		t.Fatalf("expected auto-check FAIL on stderr:\n%s", e1.String())
	}

	// --no-check: no [check] line
	var o2, e2 strings.Builder
	opts.NoCheck = true
	if err := Run(opts, IO{Out: &o2, Err: &e2}); err != nil {
		t.Fatalf("Run no-check: %v", err)
	}
	if strings.Contains(e2.String(), "[check]") {
		t.Fatalf("--no-check should suppress the check line:\n%s", e2.String())
	}
}
