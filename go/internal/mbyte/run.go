package mbyte

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// This is a faithful port of mbyte/cli.py's run()/_resolve_hex()/_resolve_output().
// The CLI layer (cmd/mbyte) only wires cobra flags into RunOptions and maps
// errors to exit codes; all behavior lives here so core.go/run.go stay the
// single source of truth (mirroring Python's core.py/cli.py split).

// RunOptions carries the resolved flag values for a single insert/replace run.
type RunOptions struct {
	Command   string // "insert" or "replace"
	InputFile string
	Output    string // -o/--output; "" means unset
	Preset    string // --preset; "" means unset
	Hex       string // --hex; "" means unset
	Force     bool
	Verbose   bool
	Raw       bool // replace only
}

// ResolveHex returns the hex string to use: the preset's hex if --preset is
// set (error if unknown), otherwise the raw --hex value. Mirrors _resolve_hex.
func ResolveHex(presetName, hexBytes string) (string, error) {
	if presetName != "" {
		hex, ok := PresetHex(presetName)
		if !ok {
			return "", newMbyteErrorf(
				"unknown preset '%s'; valid presets: %s",
				presetName, strings.Join(presetNames(), ", "),
			)
		}
		return hex, nil
	}
	return hexBytes, nil
}

// ResolveOutput computes the output path. Mirrors _resolve_output: explicit -o
// wins; else a known --preset uses its natural extension; else ".mbyte". Refuses
// to overwrite the input file unless force is set.
func ResolveOutput(inputFile, output, presetName string, force bool) (string, error) {
	var out string
	switch {
	case output != "":
		out = output
	case presetName != "":
		// --preset known -> use its natural extension, e.g. shell.php -> shell.php.png
		ext, ok := PresetExtension(presetName)
		if !ok {
			ext = "mbyte"
		}
		out = fmt.Sprintf("%s.%s", inputFile, ext)
	default:
		out = fmt.Sprintf("%s.mbyte", inputFile)
	}

	outAbs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	inAbs, err := filepath.Abs(inputFile)
	if err != nil {
		return "", err
	}
	if outAbs == inAbs && !force {
		return "", newMbyteErrorf(
			"refusing to overwrite the input file; choose a different -o/--output " +
				"path or pass --force",
		)
	}
	return out, nil
}

// Run performs a single insert/replace and prints the same confirmation output
// as cli.py's run(). It writes all normal output to w and returns an error
// (*MbyteError or one wrapping os.ErrNotExist) on failure.
func Run(opts RunOptions, w io.Writer) error {
	hexBytes, err := ResolveHex(opts.Preset, opts.Hex)
	if err != nil {
		return err
	}
	output, err := ResolveOutput(opts.InputFile, opts.Output, opts.Preset, opts.Force)
	if err != nil {
		return err
	}

	if opts.Verbose {
		fmt.Fprintf(w, "[*] command    : %s\n", opts.Command)
		fmt.Fprintf(w, "[*] input      : %s\n", opts.InputFile)
		fmt.Fprintf(w, "[*] output     : %s\n", output)
		fmt.Fprintf(w, "[*] magic hex  : %s\n", hexBytes)
	}

	magic, err := ParseHex(hexBytes)
	if err != nil {
		return err
	}

	if opts.Command == "insert" {
		if err := InsertMagicBytes(opts.InputFile, output, magic); err != nil {
			return err
		}
	} else {
		if opts.Verbose && !opts.Raw {
			if data, rerr := os.ReadFile(opts.InputFile); rerr == nil {
				head := data
				if len(head) > 16 {
					head = head[:16]
				}
				if boundary, ok := DetectSignatureLength(head); ok {
					fmt.Fprintf(w,
						"[*] detected signature length: %d bytes (replacing with %d)\n",
						boundary, len(magic),
					)
				}
			}
		}
		if err := ReplaceMagicBytes(opts.InputFile, output, magic, opts.Raw); err != nil {
			return err
		}
	}

	outData, err := os.ReadFile(output)
	if err != nil {
		return err
	}
	head := outData
	if len(head) > 16 {
		head = head[:16]
	}

	fmt.Fprintf(w, "[+] wrote %s\n", output)
	fmt.Fprintf(w, "[+] detected format: %s\n", DetectFormat(head))
	fmt.Fprintln(w, Hexdump(head, 16))
	return nil
}
