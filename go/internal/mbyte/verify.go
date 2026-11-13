package mbyte

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"debug/pe"
	"errors"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
)

// Step 6: structural (polyglot) verification. detect_format only looks at the
// leading signature; this goes further and actually tries to PARSE the file
// with a real decoder. A file that merely had a signature prepended
// (`insert --preset png` onto arbitrary bytes) matches the signature but fails
// to decode -- which is exactly the lesson: "signature matches" != "valid file".

// StructStatus is the outcome of a structural check.
type StructStatus int

const (
	StructSkip StructStatus = iota // no structural checker for this format
	StructPass                     // decoded successfully
	StructFail                     // a decoder was tried and rejected the data
)

func (s StructStatus) String() string {
	switch s {
	case StructPass:
		return "PASS"
	case StructFail:
		return "FAIL"
	default:
		return "SKIP"
	}
}

// ErrCheckFailed signals that `mbyte check` found a structural failure. The CLI
// maps it to exit 1 without printing an extra error line (the report is already
// on stdout).
var ErrCheckFailed = errors.New("structural check failed")

// errNoChecker is returned internally when a detected format has no decoder.
var errNoChecker = errors.New("no structural checker for this format")

// decodeCheck tries to fully parse data as the given detected format. It
// returns nil on success, errNoChecker if the format is unsupported, or the
// decoder's own error (kept verbatim for its learning value) on failure.
func decodeCheck(format string, data []byte) error {
	switch format {
	case "PNG image data":
		_, err := png.Decode(bytes.NewReader(data))
		return err
	case "JPEG image data":
		_, err := jpeg.Decode(bytes.NewReader(data))
		return err
	case "GIF image data, version 89a", "GIF image data, version 87a":
		_, err := gif.Decode(bytes.NewReader(data))
		return err
	case "Zip archive data":
		// zip inspects the End of Central Directory at the END of the file, so a
		// prepended signature alone can never make a valid archive -- a vivid
		// example of why a signature check is not enough.
		_, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		return err
	case "ELF executable":
		f, err := elf.NewFile(bytes.NewReader(data))
		if err == nil {
			f.Close()
		}
		return err
	case "PE32 executable (MS-DOS MZ)":
		f, err := pe.NewFile(bytes.NewReader(data))
		if err == nil {
			f.Close()
		}
		return err
	default:
		return errNoChecker
	}
}

// CheckBytes detects the format of data and tries to decode it, returning the
// status, the detected format name, and (on failure) the decoder's error text.
func CheckBytes(data []byte) (status StructStatus, format string, detail string) {
	format = DetectFormat(data)
	err := decodeCheck(format, data)
	switch {
	case err == nil:
		return StructPass, format, ""
	case errors.Is(err, errNoChecker):
		return StructSkip, format, ""
	default:
		return StructFail, format, err.Error()
	}
}

// RunCheck implements the `check` subcommand: it reports both the signature
// match (detect_format) and the structural validity of a file, as a single
// report on s.Out. It returns ErrCheckFailed when the structure is FAIL.
func RunCheck(path string, s IO) error {
	data, err := readInput(path)
	if err != nil {
		return err
	}
	head := data
	if len(head) > 16 {
		head = head[:16]
	}

	status, format, detail := CheckBytes(data)

	fmt.Fprintf(s.Out, "file: %s\n", path)
	if n, ok := DetectSignatureLength(head); ok {
		fmt.Fprintf(s.Out, "[signature] %s (matched %d bytes)\n", format, n)
	} else {
		fmt.Fprintf(s.Out, "[signature] %s\n", format)
	}
	switch status {
	case StructPass:
		fmt.Fprintf(s.Out, "[structure] PASS\n")
	case StructFail:
		fmt.Fprintf(s.Out, "[structure] FAIL (%s)\n", detail)
	default:
		fmt.Fprintf(s.Out, "[structure] SKIP (no structural checker for this format)\n")
	}

	if status == StructFail {
		return ErrCheckFailed
	}
	return nil
}

// writeAutoCheck runs a structural check on freshly written bytes and appends a
// one-line result to w (stderr). Used after insert/replace unless --no-check.
func writeAutoCheck(w io.Writer, data []byte) {
	status, format, detail := CheckBytes(data)
	switch status {
	case StructPass:
		fmt.Fprintf(w, "[check] structure: PASS (%s)\n", format)
	case StructFail:
		fmt.Fprintf(w, "[check] structure: FAIL (%s)\n", detail)
	default:
		fmt.Fprintf(w, "[check] structure: skipped (no checker for %q)\n", format)
	}
}
