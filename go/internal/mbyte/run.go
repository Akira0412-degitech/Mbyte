package mbyte

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// This is a faithful port of mbyte/cli.py's run()/_resolve_hex()/_resolve_output(),
// extended in Step 4 with the payload-hash preview and the pre-write
// confirmation flow. The CLI layer (cmd/mbyte) only wires cobra flags into
// RunOptions + IO and maps errors to exit codes; all behavior lives here so
// core.go/run.go stay the single source of truth.

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
	Yes       bool // -y/--yes: skip the confirmation prompt
	DryRun    bool // --dry-run: show the preview and exit without writing
	Recursive bool // -r/--recursive: recurse when the input is a directory (batch)
	JSON      bool // --json: print batch results as a JSON array
	NoCheck   bool // --no-check: skip the post-write structural check
}

// IO bundles the streams and interactivity for a run so output and prompting
// are injectable (and therefore testable). Results go to Out (stdout); the
// preview, warnings and the confirmation prompt go to Err (stderr); prompt
// answers are read from In. Interactive reports whether we may prompt at all
// (true only on a real terminal).
type IO struct {
	Out         io.Writer
	Err         io.Writer
	In          io.Reader
	Interactive bool
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

// preview holds everything the pre-write report needs. Building it does not
// touch the output file, so it works for --dry-run too.
type preview struct {
	command    string
	inputFile  string
	output     string
	magic      []byte
	original   []byte
	outData    []byte
	boundaryOld int    // old signature length, or -1 if unrecognized
	boundaryNew int    // new signature length (== len(magic))
	hashBefore string // "" if the before-payload is unknown (raw + unrecognized)
	hashAfter  string
	status     string // full text, e.g. "MISMATCH (payload shifted by +4 bytes)"
	statusCode string // bare token: "MATCH", "MISMATCH", or "UNVERIFIED"
	ok         bool   // true for MATCH
}

// buildPreview computes the output bytes and the payload-hash comparison for a
// run, without writing anything.
func buildPreview(opts RunOptions, output string, magic []byte, original []byte) (*preview, error) {
	p := &preview{
		command:     opts.Command,
		inputFile:   opts.InputFile,
		output:      output,
		magic:       magic,
		original:    original,
		boundaryNew: len(magic),
		boundaryOld: -1,
	}

	if opts.Command == "insert" {
		// The whole original file is payload (boundary 0); insert must keep it
		// byte-for-byte, so the after-payload is original again -> always MATCH.
		p.outData = buildInserted(original, magic)
		p.boundaryOld = 0
		p.hashBefore = HashPayload(original, 0)
		p.hashAfter = HashPayload(p.outData, len(magic))
	} else {
		out, boundaryOld, err := buildReplaced(original, magic, opts.Raw)
		if err != nil {
			return nil, err
		}
		p.outData = out
		p.boundaryOld = boundaryOld
		if boundaryOld >= 0 {
			p.hashBefore = HashPayload(original, boundaryOld)
		}
		p.hashAfter = HashPayload(out, len(magic))
	}

	switch {
	case p.boundaryOld < 0:
		// Only reachable for replace --raw on an unrecognized current signature:
		// we cannot locate the real payload start, so we cannot verify it.
		p.statusCode = "UNVERIFIED"
		p.status = "UNVERIFIED (current signature unrecognized; cannot locate payload start)"
	case p.hashBefore == p.hashAfter:
		p.statusCode = "MATCH"
		p.status = "MATCH"
		p.ok = true
	default:
		shift := p.boundaryOld - len(magic)
		p.statusCode = "MISMATCH"
		p.status = fmt.Sprintf("MISMATCH (payload shifted by %+d bytes)", shift)
	}
	return p, nil
}

// signatureDesc formats a signature line like `89504e47 ("PNG image data", 4 bytes)`.
func signatureDesc(sig []byte) string {
	return fmt.Sprintf("%x (%q, %d bytes)", sig, DetectFormat(sig), len(sig))
}

// writePreview prints the full [preview] block to w (stderr).
func (p *preview) writePreview(w io.Writer) {
	fmt.Fprintf(w, "[preview] input    : %s (%d bytes)\n", p.inputFile, len(p.original))
	if p.command == "insert" {
		fmt.Fprintf(w, "[preview] action   : insert %s at start\n", signatureDesc(p.magic))
	} else {
		if p.boundaryOld >= 0 {
			fmt.Fprintf(w, "[preview] current signature : %s\n", signatureDesc(p.original[:p.boundaryOld]))
		} else {
			fmt.Fprintf(w, "[preview] current signature : (unrecognized)\n")
		}
		fmt.Fprintf(w, "[preview] new signature     : %s\n", signatureDesc(p.magic))
	}
	before := p.hashBefore
	if before == "" {
		before = "n/a"
	}
	fmt.Fprintf(w, "[preview] payload hash (before)           : %s\n", before)
	fmt.Fprintf(w, "[preview] payload hash (after, estimated) : %s [%s]\n", p.hashAfter, p.status)
	fmt.Fprintf(w, "[preview] output   : %s (%d bytes, %+d bytes)\n",
		p.output, len(p.outData), len(p.outData)-len(p.original))
}

// confirm prints a [y/N] prompt to w and reads one line from r. Only "y"/"yes"
// (case-insensitive) proceeds; the default is No.
func confirm(r io.Reader, w io.Writer, output string) bool {
	fmt.Fprintf(w, "[?] proceed and write %s? [y/N]: ", output)
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// Run performs a single insert/replace. Results (the cli.py confirmation lines)
// go to s.Out; the Step 4 preview/warnings/prompt go to s.Err. It returns an
// error (*MbyteError or one wrapping os.ErrNotExist) on failure. A user
// declining the prompt, or --dry-run, is a normal (nil) outcome with no write.
func Run(opts RunOptions, s IO) error {
	hexBytes, err := ResolveHex(opts.Preset, opts.Hex)
	if err != nil {
		return err
	}
	output, err := ResolveOutput(opts.InputFile, opts.Output, opts.Preset, opts.Force)
	if err != nil {
		return err
	}
	magic, err := ParseHex(hexBytes)
	if err != nil {
		return err
	}

	// Read + build in memory so the preview and the write see the same bytes.
	original, err := readInput(opts.InputFile)
	if err != nil {
		return err
	}
	p, err := buildPreview(opts, output, magic, original)
	if err != nil {
		return err
	}

	// stdout verbose lines (kept identical to Step 2 / cli.py for parity).
	if opts.Verbose {
		fmt.Fprintf(s.Out, "[*] command    : %s\n", opts.Command)
		fmt.Fprintf(s.Out, "[*] input      : %s\n", opts.InputFile)
		fmt.Fprintf(s.Out, "[*] output     : %s\n", output)
		fmt.Fprintf(s.Out, "[*] magic hex  : %s\n", hexBytes)
		if opts.Command == "replace" && !opts.Raw && p.boundaryOld >= 0 {
			fmt.Fprintf(s.Out,
				"[*] detected signature length: %d bytes (replacing with %d)\n",
				p.boundaryOld, len(magic),
			)
		}
	}

	// stderr preview / warnings. Full preview when the user asked to see it
	// (--dry-run, -v) or we are about to prompt; otherwise surface only a
	// problem (MISMATCH/UNVERIFIED) so happy-path runs stay quiet.
	showPreview := opts.DryRun || opts.Verbose || s.Interactive
	if showPreview {
		p.writePreview(s.Err)
	} else if !p.ok {
		fmt.Fprintf(s.Err, "[!] payload %s\n", p.status)
	}

	if opts.DryRun {
		fmt.Fprintln(s.Err, "[*] dry-run: no file written")
		return nil
	}

	if s.Interactive && !opts.Yes {
		if !confirm(s.In, s.Err, output) {
			fmt.Fprintln(s.Err, "[*] aborted: no file written")
			return nil
		}
	}

	if err := os.WriteFile(output, p.outData, 0o644); err != nil {
		return err
	}

	head := p.outData
	if len(head) > 16 {
		head = head[:16]
	}
	fmt.Fprintf(s.Out, "[+] wrote %s\n", output)
	fmt.Fprintf(s.Out, "[+] detected format: %s\n", DetectFormat(head))
	fmt.Fprintln(s.Out, Hexdump(head, 16))

	// Post-write structural check (Step 6): prove whether the file is actually
	// parseable as its claimed format, not just signature-matching. Goes to
	// stderr so stdout stays the clean result; disabled by --no-check.
	if !opts.NoCheck {
		writeAutoCheck(s.Err, p.outData)
	}
	return nil
}
