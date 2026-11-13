package mbyte

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPresetHexValues pins the hex each preset writes, including the Step 3
// additions and the jpeg special case (writes ffd8ffe0, not the 3-byte
// detection prefix).
func TestPresetHexValues(t *testing.T) {
	cases := map[string]string{
		"png":       "89504e470d0a1a0a",
		"gif89a":    "474946383961",
		"gif87a":    "474946383761",
		"jpeg":      "ffd8ffe0",
		"pdf":       "25504446",
		"zip":       "504b0304",
		"elf":       "7f454c46",
		"pe":        "4d5a",
		"gzip":      "1f8b",
		"7z":        "377abcaf271c",
		"rar":       "52617221",
		"javaclass": "cafebabe",
	}
	for name, want := range cases {
		got, ok := PresetHex(name)
		if !ok {
			t.Errorf("PresetHex(%q): ok=false, want preset to exist", name)
			continue
		}
		if got != want {
			t.Errorf("PresetHex(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestPresetExtensions pins the default output extension for each preset.
func TestPresetExtensions(t *testing.T) {
	cases := map[string]string{
		"png":       "png",
		"gif89a":    "gif",
		"gif87a":    "gif",
		"jpeg":      "jpg",
		"pdf":       "pdf",
		"zip":       "zip",
		"elf":       "elf",
		"pe":        "exe",
		"gzip":      "gz",
		"7z":        "7z",
		"rar":       "rar",
		"javaclass": "class",
	}
	for name, want := range cases {
		got, ok := PresetExtension(name)
		if !ok {
			t.Errorf("PresetExtension(%q): ok=false", name)
			continue
		}
		if got != want {
			t.Errorf("PresetExtension(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestBMPNotAPreset guards that the detection-only BMP signature is never
// exposed as an insert preset (Step 3 scope decision).
func TestBMPNotAPreset(t *testing.T) {
	if _, ok := PresetHex("bmp"); ok {
		t.Fatal("bmp should be detection-only, not an insert preset")
	}
}

// TestPresetCount checks the table exposes exactly the expected 12 presets.
func TestPresetCount(t *testing.T) {
	if n := len(presetNames()); n != 12 {
		t.Fatalf("presetNames() has %d entries, want 12", n)
	}
}

// TestInsertPresetRoundTrips is the core Step 3 DoD: inserting each preset's
// magic bytes produces a file whose leading bytes DetectFormat re-recognizes.
func TestInsertPresetRoundTrips(t *testing.T) {
	// Expected detect_format Name after inserting each preset.
	wantName := map[string]string{
		"png":       "PNG image data",
		"gif89a":    "GIF image data, version 89a",
		"gif87a":    "GIF image data, version 87a",
		"jpeg":      "JPEG image data",
		"pdf":       "PDF document",
		"zip":       "Zip archive data",
		"elf":       "ELF executable",
		"pe":        "PE32 executable (MS-DOS MZ)",
		"gzip":      "gzip compressed data",
		"7z":        "7-zip archive data",
		"rar":       "RAR archive data",
		"javaclass": "Java class data",
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(in, []byte("payload\x00data"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	for _, name := range presetNames() {
		hexStr, _ := PresetHex(name)
		magic, err := ParseHex(hexStr)
		if err != nil {
			t.Fatalf("ParseHex(%s=%q): %v", name, hexStr, err)
		}
		out := filepath.Join(dir, name+".out")
		if err := InsertMagicBytes(in, out, magic); err != nil {
			t.Fatalf("InsertMagicBytes(%s): %v", name, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("read output(%s): %v", name, err)
		}
		if got := DetectFormat(data); got != wantName[name] {
			t.Errorf("after insert --preset %s: DetectFormat = %q, want %q", name, got, wantName[name])
		}
	}
}
