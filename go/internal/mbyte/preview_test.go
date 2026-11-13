package mbyte

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// known SHA-256 of "abc".
const sha256abc = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func TestHashPayload(t *testing.T) {
	// boundary 0: whole input.
	if got := HashPayload([]byte("abc"), 0); got != sha256abc {
		t.Errorf("HashPayload(abc,0) = %s, want %s", got, sha256abc)
	}
	// boundary skips the signature so only "abc" is hashed.
	if got := HashPayload([]byte("XYabc"), 2); got != sha256abc {
		t.Errorf("HashPayload(XYabc,2) = %s, want %s", got, sha256abc)
	}
	// out-of-range boundaries clamp (empty payload -> sha256 of "").
	emptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := HashPayload([]byte("abc"), 99); got != emptyHash {
		t.Errorf("HashPayload clamp = %s, want empty-hash", got)
	}
}

// pngThenRest builds a file whose leading signature is PNG (8 bytes) followed by
// some payload, and returns its path.
func pngInput(t *testing.T, dir string) string {
	t.Helper()
	data := append(decodeHex(t, "89504e470d0a1a0a"), []byte("REST-PAYLOAD")...)
	p := filepath.Join(dir, "in.png")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	return p
}

// TestRunReplaceBoundaryMatch: a boundary-detected replace preserves the
// payload, so the preview reports [MATCH].
func TestRunReplaceBoundaryMatch(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir)
	out := filepath.Join(dir, "out.gif")

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "replace", InputFile: in, Output: out, Preset: "gif89a", DryRun: true}
	if err := Run(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errBuf.String(), "[MATCH]") {
		t.Fatalf("preview missing [MATCH]:\n%s", errBuf.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write the output file")
	}
}

// TestRunReplaceRawMismatch: a raw replace whose length differs from the real
// signature shifts the payload, so the run reports MISMATCH (even without a
// preview) and still writes.
func TestRunReplaceRawMismatch(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir) // PNG signature is 8 bytes
	out := filepath.Join(dir, "out.jpg")

	var outBuf, errBuf strings.Builder
	// 4-byte magic via --raw: new boundary 4 vs detected 8 -> shift +4 bytes.
	opts := RunOptions{Command: "replace", InputFile: in, Output: out, Hex: "ffd8ffe0", Raw: true}
	if err := Run(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errBuf.String(), "MISMATCH") {
		t.Fatalf("expected MISMATCH warning, got:\n%s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "+4 bytes") {
		t.Fatalf("expected shift of +4 bytes, got:\n%s", errBuf.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("non-dry-run should write the output file: %v", err)
	}
}

// TestRunDryRunNoWrite: --dry-run prints the preview and writes nothing.
func TestRunDryRunNoWrite(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir)
	out := filepath.Join(dir, "out.gif")

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "gif89a", DryRun: true}
	if err := Run(opts, IO{Out: &outBuf, Err: &errBuf}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errBuf.String(), "dry-run: no file written") {
		t.Fatalf("missing dry-run notice:\n%s", errBuf.String())
	}
	if outBuf.Len() != 0 {
		t.Fatalf("dry-run must not print result lines to Out:\n%s", outBuf.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create the output file")
	}
}

// TestRunConfirmDeclined: an interactive run answered "n" aborts without writing.
func TestRunConfirmDeclined(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir)
	out := filepath.Join(dir, "out.gif")

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "gif89a"}
	s := IO{Out: &outBuf, Err: &errBuf, In: strings.NewReader("n\n"), Interactive: true}
	if err := Run(opts, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errBuf.String(), "aborted: no file written") {
		t.Fatalf("missing abort notice:\n%s", errBuf.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("declined confirmation must not write the output file")
	}
}

// TestRunConfirmAccepted: an interactive run answered "y" writes the file.
func TestRunConfirmAccepted(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir)
	out := filepath.Join(dir, "out.gif")

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "gif89a"}
	s := IO{Out: &outBuf, Err: &errBuf, In: strings.NewReader("y\n"), Interactive: true}
	if err := Run(opts, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(outBuf.String(), "[+] wrote ") {
		t.Fatalf("accepted confirmation should write and report:\n%s", outBuf.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("accepted confirmation must create the output file: %v", err)
	}
}

// TestRunYesSkipsPrompt: -y proceeds without reading from In even on an
// interactive stream.
func TestRunYesSkipsPrompt(t *testing.T) {
	dir := t.TempDir()
	in := pngInput(t, dir)
	out := filepath.Join(dir, "out.gif")

	var outBuf, errBuf strings.Builder
	opts := RunOptions{Command: "insert", InputFile: in, Output: out, Preset: "gif89a", Yes: true}
	// In is nil on purpose: --yes must not try to read a prompt answer.
	s := IO{Out: &outBuf, Err: &errBuf, Interactive: true}
	if err := Run(opts, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(outBuf.String(), "[+] wrote ") {
		t.Fatalf("-y should write without prompting:\n%s", outBuf.String())
	}
}
