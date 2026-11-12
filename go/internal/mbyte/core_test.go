package mbyte

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Signature hex values mirrored from core.py / presets.py so the Go tests
// assert the same bytes. The preset table itself is unified in Step 3; here we
// keep local literals (test_preset_values_are_known is deferred to Step 3).
const (
	hexPNG    = "89504e470d0a1a0a" // 8 bytes
	hexGIF89a = "474946383961"     // 6 bytes
	hexJPEG   = "ffd8ffe0"         // 4-byte preset; detection matches 3 bytes
	hexPDF    = "25504446"
	hexZIP    = "504b0304"
)

func decodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad test hex %q: %v", s, err)
	}
	return b
}

// write creates a file under dir with the given bytes and returns its path.
func write(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func requireMbyteError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an MbyteError, got nil")
	}
	var me *MbyteError
	if !errors.As(err, &me) {
		t.Fatalf("expected *MbyteError, got %T: %v", err, err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func TestInsertPrependsAndPreserves(t *testing.T) {
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), []byte("hello"))
	out := filepath.Join(dir, "out.bin")

	if err := InsertMagicBytes(src, out, decodeHex(t, "89504e47")); err != nil {
		t.Fatalf("insert: %v", err)
	}

	want := append(decodeHex(t, "89504e47"), []byte("hello")...)
	if got := readFile(t, out); !bytes.Equal(got, want) {
		t.Fatalf("output = %x, want %x", got, want)
	}
	// input untouched
	if got := readFile(t, src); !bytes.Equal(got, []byte("hello")) {
		t.Fatalf("input mutated: %x", got)
	}
}

func TestReplaceRawOverwritesHeadKeepsTail(t *testing.T) {
	// "ABCDEFGH" has no recognized signature, so the fixed-length overwrite
	// path (raw=true) is the right fit for this behavior.
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), []byte("ABCDEFGH"))
	out := filepath.Join(dir, "out.bin")

	if err := ReplaceMagicBytes(src, out, decodeHex(t, "00112233"), true); err != nil { // 4 bytes
		t.Fatalf("replace: %v", err)
	}

	want := append(decodeHex(t, "00112233"), []byte("EFGH")...)
	if got := readFile(t, out); !bytes.Equal(got, want) {
		t.Fatalf("output = %x, want %x", got, want)
	}
}

func TestParseHexRejectsInvalid(t *testing.T) {
	for _, bad := range []string{"abc", "zz", "89504e4g", "12 3"} {
		bad := bad
		t.Run(bad, func(t *testing.T) {
			_, err := ParseHex(bad)
			requireMbyteError(t, err)
		})
	}
}

// Maps test_invalid_hex_in_insert_raises: since the core now takes already
// parsed bytes, the invalid-hex guard lives in ParseHex (called by the CLI).
func TestParseHexRejectsNonHexWord(t *testing.T) {
	_, err := ParseHex("xyz")
	requireMbyteError(t, err)
}

func TestReplaceTooShortRaises(t *testing.T) {
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), []byte("ab")) // 2 bytes
	out := filepath.Join(dir, "out.bin")

	// raw=true exercises the fixed-length too-short guard directly (8 bytes).
	err := ReplaceMagicBytes(src, out, decodeHex(t, "89504e470d0a1a0a"), true)
	requireMbyteError(t, err)
}

func TestMissingInputRaises(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")

	err := InsertMagicBytes(filepath.Join(dir, "nope.bin"), out, decodeHex(t, "89504e47"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %T: %v", err, err)
	}
}

func TestDetectFormatPNG(t *testing.T) {
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), []byte("hello"))
	out := filepath.Join(dir, "out.png")

	if err := InsertMagicBytes(src, out, decodeHex(t, hexPNG)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := DetectFormat(readFile(t, out)); got != "PNG image data" {
		t.Fatalf("detect = %q, want PNG image data", got)
	}
}

func TestReplaceWithShorterMagicSplicesAtDetectedBoundary(t *testing.T) {
	// New signature shorter than the old one: detection (default) splices at
	// the OLD signature's real end, so no leftover old-signature bytes survive.
	// PNG signature is 8 bytes; GIF89a is only 6. Boundary detected = 8 (PNG).
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), append(decodeHex(t, hexPNG), []byte("REST")...))
	out := filepath.Join(dir, "out.bin")

	if err := ReplaceMagicBytes(src, out, decodeHex(t, hexGIF89a), false); err != nil {
		t.Fatalf("replace: %v", err)
	}

	want := append(decodeHex(t, hexGIF89a), []byte("REST")...)
	got := readFile(t, out)
	if !bytes.Equal(got, want) {
		t.Fatalf("output = %x, want %x", got, want)
	}
	if f := DetectFormat(got); f != "GIF image data, version 89a" {
		t.Fatalf("detect = %q", f)
	}
}

func TestReplaceWithLongerMagicPreservesFollowingData(t *testing.T) {
	// Inverse case: new signature longer than the old one. Detection splices at
	// the old 6-byte boundary, so the full original tail survives untouched.
	// GIF89a signature is 6 bytes; PNG is 8. Boundary detected = 6 (GIF).
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), append(decodeHex(t, hexGIF89a), []byte("REST")...))
	out := filepath.Join(dir, "out.bin")

	if err := ReplaceMagicBytes(src, out, decodeHex(t, hexPNG), false); err != nil {
		t.Fatalf("replace: %v", err)
	}

	want := append(decodeHex(t, hexPNG), []byte("REST")...)
	got := readFile(t, out)
	if !bytes.Equal(got, want) {
		t.Fatalf("output = %x, want %x", got, want)
	}
	if f := DetectFormat(got); f != "PNG image data" {
		t.Fatalf("detect = %q", f)
	}
}

func TestReplaceRawReproducesOldFixedLengthBehavior(t *testing.T) {
	// raw=true is the explicit escape hatch: overwrite exactly len(magic) bytes
	// from the start, reproducing the old (lossy) behavior on purpose.
	dir := t.TempDir()

	// shorter new signature: leftover old PNG bytes survive after the new GIF.
	srcS := write(t, filepath.Join(dir, "s.bin"), append(decodeHex(t, hexPNG), []byte("REST")...))
	outS := filepath.Join(dir, "s.out")
	if err := ReplaceMagicBytes(srcS, outS, decodeHex(t, hexGIF89a), true); err != nil {
		t.Fatalf("replace raw (short): %v", err)
	}
	wantS := append(append(decodeHex(t, hexGIF89a), decodeHex(t, hexPNG)[6:]...), []byte("REST")...)
	if got := readFile(t, outS); !bytes.Equal(got, wantS) {
		t.Fatalf("short raw output = %x, want %x", got, wantS)
	}

	// longer new signature: the first 2 bytes of the tail get eaten.
	srcL := write(t, filepath.Join(dir, "l.bin"), append(decodeHex(t, hexGIF89a), []byte("REST")...))
	outL := filepath.Join(dir, "l.out")
	if err := ReplaceMagicBytes(srcL, outL, decodeHex(t, hexPNG), true); err != nil {
		t.Fatalf("replace raw (long): %v", err)
	}
	wantL := append(decodeHex(t, hexPNG), []byte("ST")...)
	if got := readFile(t, outL); !bytes.Equal(got, wantL) {
		t.Fatalf("long raw output = %x, want %x", got, wantL)
	}
}

func TestReplaceUnrecognizedFormatRequiresRaw(t *testing.T) {
	// Without raw, an unrecognized current signature has no safe splice point
	// and errors; with raw=true it falls back to the old fixed-length overwrite.
	dir := t.TempDir()
	src := write(t, filepath.Join(dir, "in.bin"), []byte("NOTAREALSIG"))
	out := filepath.Join(dir, "out.bin")

	err := ReplaceMagicBytes(src, out, decodeHex(t, hexPNG), false)
	requireMbyteError(t, err)

	// raw=true forces the fixed-length overwrite and succeeds.
	if err := ReplaceMagicBytes(src, out, decodeHex(t, hexPNG), true); err != nil {
		t.Fatalf("replace raw: %v", err)
	}
	magic := decodeHex(t, hexPNG)
	want := append(magic, []byte("NOTAREALSIG")[len(magic):]...)
	if got := readFile(t, out); !bytes.Equal(got, want) {
		t.Fatalf("output = %x, want %x", got, want)
	}
}

func TestDetectSignatureLength(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantLen int
		wantOk  bool
	}{
		{"png", append(decodeHex(t, hexPNG), []byte("REST")...), 8, true},
		{"gif89a", append(decodeHex(t, hexGIF89a), []byte("REST")...), 6, true},
		// jpeg detection matches the generic 3-byte FF D8 FF prefix,
		// one byte short of the 4-byte ffd8ffe0 preset marker.
		{"jpeg", append(decodeHex(t, hexJPEG), []byte("REST")...), 3, true},
		{"unknown", []byte("NOTAREALSIG"), 0, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			gotLen, gotOk := DetectSignatureLength(c.data)
			if gotOk != c.wantOk || gotLen != c.wantLen {
				t.Fatalf("DetectSignatureLength = (%d, %v), want (%d, %v)",
					gotLen, gotOk, c.wantLen, c.wantOk)
			}
		})
	}
}
