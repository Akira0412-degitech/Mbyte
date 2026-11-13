package mbyte

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHexPreset(t *testing.T) {
	got, err := ResolveHex("png", "")
	if err != nil {
		t.Fatalf("ResolveHex(png): %v", err)
	}
	if got != "89504e470d0a1a0a" {
		t.Fatalf("ResolveHex(png) = %q", got)
	}
}

func TestResolveHexRawPassthrough(t *testing.T) {
	got, err := ResolveHex("", "ffd8ffe0")
	if err != nil {
		t.Fatalf("ResolveHex(hex): %v", err)
	}
	if got != "ffd8ffe0" {
		t.Fatalf("ResolveHex(hex) = %q", got)
	}
}

func TestResolveHexUnknownPreset(t *testing.T) {
	// bmp is detection-only (not an insert preset), so it is still unknown here.
	_, err := ResolveHex("bmp", "")
	requireMbyteError(t, err)
	// error lists the valid presets in table order, like cli.py (now 12 after
	// the Step 3 unification added elf/pe/gzip/7z/rar/javaclass).
	if !strings.Contains(err.Error(),
		"valid presets: png, gif89a, gif87a, jpeg, pdf, zip, elf, pe, gzip, 7z, rar, javaclass") {
		t.Fatalf("unexpected message: %v", err)
	}
}

func TestResolveOutputExplicit(t *testing.T) {
	got, err := ResolveOutput("in.bin", "out.png", "png", false)
	if err != nil {
		t.Fatalf("ResolveOutput: %v", err)
	}
	if got != "out.png" {
		t.Fatalf("got %q, want out.png", got)
	}
}

func TestResolveOutputPresetExtension(t *testing.T) {
	got, err := ResolveOutput("shell.php", "", "png", false)
	if err != nil {
		t.Fatalf("ResolveOutput: %v", err)
	}
	if got != "shell.php.png" {
		t.Fatalf("got %q, want shell.php.png", got)
	}
}

func TestResolveOutputDefaultMbyte(t *testing.T) {
	got, err := ResolveOutput("notes.txt", "", "", false)
	if err != nil {
		t.Fatalf("ResolveOutput: %v", err)
	}
	if got != "notes.txt.mbyte" {
		t.Fatalf("got %q, want notes.txt.mbyte", got)
	}
}

func TestResolveOutputRefusesOverwriteWithoutForce(t *testing.T) {
	_, err := ResolveOutput("same.bin", "same.bin", "", false)
	requireMbyteError(t, err)
}

func TestResolveOutputOverwriteAllowedWithForce(t *testing.T) {
	got, err := ResolveOutput("same.bin", "same.bin", "", true)
	if err != nil {
		t.Fatalf("ResolveOutput(force): %v", err)
	}
	if got != "same.bin" {
		t.Fatalf("got %q, want same.bin", got)
	}
}

func TestListPresetsExactFormat(t *testing.T) {
	// NAME column is width-aligned to the longest name, now "javaclass" (9).
	want := strings.Join([]string{
		"Available presets:",
		"",
		"  NAME       HEX",
		"  ---------  ----------------",
		"  png        89504e470d0a1a0a",
		"  gif89a     474946383961",
		"  gif87a     474946383761",
		"  jpeg       ffd8ffe0",
		"  pdf        25504446",
		"  zip        504b0304",
		"  elf        7f454c46",
		"  pe         4d5a",
		"  gzip       1f8b",
		"  7z         377abcaf271c",
		"  rar        52617221",
		"  javaclass  cafebabe",
	}, "\n")
	if got := ListPresets(); got != want {
		t.Fatalf("ListPresets mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRunInsertOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(in, append([]byte("payload"), 0x00), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := filepath.Join(dir, "payload.png")

	var buf bytes.Buffer
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "png"}
	// Non-interactive: no prompt, results on Out.
	if err := Run(opts, IO{Out: &buf, Err: io.Discard}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := buf.String()
	// The exact confirmation lines cli.py prints (hexdump reproduces README).
	for _, want := range []string{
		"[+] wrote " + out,
		"[+] detected format: PNG image data",
		"00000000: 89 50 4e 47 0d 0a 1a 0a 70 61 79 6c 6f 61 64 00  .PNG....payload.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q\n--- got ---\n%s", want, got)
		}
	}
}
