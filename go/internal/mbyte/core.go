// Package mbyte holds the core insert/replace logic and helpers, ported
// faithfully from the Python reference implementation (mbyte/core.py).
//
// Functions here return errors on bad input: *MbyteError for invalid user
// input or impossible operations, and errors wrapping os.ErrNotExist for a
// missing input file (mirroring Python's MbyteError / FileNotFoundError split).
// The CLI layer is responsible for turning those into friendly messages and
// exit codes. This keeps the core easy to unit-test.
package mbyte

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// MbyteError is raised for invalid user input or impossible operations.
// It mirrors the Python MbyteError exception (a single class covering all
// user-input / impossible-operation cases).
type MbyteError struct {
	Msg string
}

func (e *MbyteError) Error() string { return e.Msg }

func newMbyteErrorf(format string, a ...any) *MbyteError {
	return &MbyteError{Msg: fmt.Sprintf(format, a...)}
}

// Signature maps a magic-byte prefix to a human-readable format name.
// (Step 3 will extend this struct with PresetName/Ext; not added yet so the
// port stays 1:1 with core.py.)
type Signature struct {
	Hex  []byte
	Name string
}

// mustHex decodes a signature literal at init time; the inputs are constants,
// so a decode failure is a programming error.
func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid signature literal: " + s)
	}
	return b
}

// signatures maps a signature prefix to a human-readable format name
// (file(1)-ish), longest match wins. Same order and names as core.py's
// _SIGNATURES.
var signatures = []Signature{
	{mustHex("89504e470d0a1a0a"), "PNG image data"},
	{mustHex("474946383961"), "GIF image data, version 89a"},
	{mustHex("474946383761"), "GIF image data, version 87a"},
	// JPEG is matched on the generic 3-byte FF D8 FF prefix so that both JFIF
	// (ffd8ffe0) and EXIF (ffd8ffe1) variants are recognized. This means the
	// detected JPEG boundary is intentionally 3 bytes -- one byte short of the
	// 4-byte APP0/APP1 marker that the "jpeg" preset writes -- so a `replace`
	// onto a JPEG may leave that 4th, marker-ambiguous byte behind. Documented
	// limitation, not a bug: we only ever claim the 3 bytes we actually know.
	{mustHex("ffd8ff"), "JPEG image data"},
	{mustHex("25504446"), "PDF document"},
	{mustHex("504b0304"), "Zip archive data"},
	{mustHex("424d"), "PC bitmap (BMP) image data"},
	{mustHex("7f454c46"), "ELF executable"},
	{mustHex("4d5a"), "PE32 executable (MS-DOS MZ)"},
	{mustHex("1f8b"), "gzip compressed data"},
	{mustHex("377abcaf271c"), "7-zip archive data"},
	{mustHex("52617221"), "RAR archive data"},
	{mustHex("cafebabe"), "Java class data"},
}

const hexDigits = "0123456789abcdef"

// ParseHex validates and converts a hex string to bytes.
//
// Returns an *MbyteError for an empty string, odd length, or non-hex
// characters.
func ParseHex(hexBytes string) ([]byte, error) {
	cleaned := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(hexBytes)), " ", "")
	if cleaned == "" {
		return nil, newMbyteErrorf("hex string is empty")
	}
	if len(cleaned)%2 != 0 {
		return nil, newMbyteErrorf(
			"hex string has an odd number of digits (%d): %q", len(cleaned), cleaned,
		)
	}
	badSet := map[rune]struct{}{}
	for _, c := range cleaned {
		if !strings.ContainsRune(hexDigits, c) {
			badSet[c] = struct{}{}
		}
	}
	if len(badSet) > 0 {
		bad := make([]string, 0, len(badSet))
		for c := range badSet {
			bad = append(bad, string(c))
		}
		sort.Strings(bad)
		return nil, newMbyteErrorf(
			"hex string contains invalid characters %v: %q", bad, cleaned,
		)
	}
	b, err := hex.DecodeString(cleaned)
	if err != nil {
		// Should be unreachable given the checks above, but stay safe.
		return nil, newMbyteErrorf("invalid hex string %q: %v", cleaned, err)
	}
	return b, nil
}

func readInput(inputPath string) ([]byte, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("input file not found: %s: %w", inputPath, os.ErrNotExist)
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, newMbyteErrorf("input path is a directory, not a file: %s", inputPath)
	}
	return os.ReadFile(inputPath)
}

// InsertMagicBytes prepends magic to the input file, writing the result to
// outputPath. The input file is left unchanged.
func InsertMagicBytes(inputPath, outputPath string, magic []byte) error {
	original, err := readInput(inputPath)
	if err != nil {
		return err
	}
	out := make([]byte, 0, len(magic)+len(original))
	out = append(out, magic...)
	out = append(out, original...)
	return os.WriteFile(outputPath, out, 0o644)
}

// ReplaceMagicBytes replaces the input file's leading signature with magic.
//
// By default (raw=false) the length of the file's EXISTING signature is
// detected via DetectSignatureLength and the splice happens at that boundary:
// output = magic + original[boundary:]. This is correct whether the new
// signature is shorter, longer, or the same length as the old one, because the
// cut point is where the old signature actually ends -- not how long the new
// one happens to be. If the current format isn't recognized, an *MbyteError is
// returned (pass raw=true to force a fixed-length overwrite, or use
// InsertMagicBytes to just prepend bytes).
//
// With raw=true the old, fixed-length behavior is used: overwrite exactly
// len(magic) bytes from the start and keep the rest. This may corrupt data or
// leave garbage bytes if the length doesn't match the file's real signature;
// it returns an *MbyteError if the input file is shorter than the replacement
// bytes.
func ReplaceMagicBytes(inputPath, outputPath string, magic []byte, raw bool) error {
	original, err := readInput(inputPath)
	if err != nil {
		return err
	}

	var output []byte
	if raw {
		if len(original) < len(magic) {
			return newMbyteErrorf(
				"input file is too short (%d bytes) to replace %d bytes; use 'insert' instead",
				len(original), len(magic),
			)
		}
		output = make([]byte, 0, len(original))
		output = append(output, magic...)
		output = append(output, original[len(magic):]...)
	} else {
		boundary, ok := DetectSignatureLength(original)
		if !ok {
			return newMbyteErrorf(
				"could not recognize the current file's signature, so there is " +
					"no safe splice point for 'replace'; pass raw=true (CLI: --raw) " +
					"to force a fixed-length overwrite, or use 'insert' to just " +
					"prepend bytes",
			)
		}
		output = make([]byte, 0, len(magic)+len(original)-boundary)
		output = append(output, magic...)
		output = append(output, original[boundary:]...)
	}

	return os.WriteFile(outputPath, output, 0o644)
}

// matchSignature returns the longest signatures entry that is a prefix of data,
// or ok=false if nothing matches.
//
// Longest-match-wins avoids a short generic prefix shadowing a more specific
// signature, and gives DetectFormat and DetectSignatureLength a single source
// of truth so they can't disagree about what matched.
func matchSignature(data []byte) (sig []byte, name string, ok bool) {
	var best Signature
	found := false
	for _, s := range signatures {
		if bytes.HasPrefix(data, s.Hex) && (!found || len(s.Hex) > len(best.Hex)) {
			best = s
			found = true
		}
	}
	if !found {
		return nil, "", false
	}
	return best.Hex, best.Name, true
}

// DetectFormat guesses a file(1)-style format name from the leading bytes.
func DetectFormat(data []byte) string {
	_, name, ok := matchSignature(data)
	if !ok {
		return "data (unknown / unrecognized signature)"
	}
	return name
}

// DetectSignatureLength returns the byte-length of the longest known signature
// that matches the start of data, and ok=false if nothing matches.
func DetectSignatureLength(data []byte) (length int, ok bool) {
	sig, _, matched := matchSignature(data)
	if !matched {
		return 0, false
	}
	return len(sig), true
}

// Hexdump returns an xxd-style dump of up to length bytes of data, byte-for-byte
// identical to the Python reference output.
func Hexdump(data []byte, length int) string {
	chunk := data
	if len(chunk) > length {
		chunk = chunk[:length]
	}
	parts := make([]string, len(chunk))
	for i, b := range chunk {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	hexPart := strings.Join(parts, " ")
	// pad hex column so the ascii gutter lines up (3 chars per byte)
	if width := length*3 - 1; len(hexPart) < width {
		hexPart += strings.Repeat(" ", width-len(hexPart))
	}
	var ascii strings.Builder
	for _, b := range chunk {
		if b >= 32 && b < 127 {
			ascii.WriteByte(b)
		} else {
			ascii.WriteByte('.')
		}
	}
	return fmt.Sprintf("00000000: %s  %s", hexPart, ascii.String())
}
