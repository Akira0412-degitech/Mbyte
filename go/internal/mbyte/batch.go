package mbyte

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Step 5: apply insert/replace to every file in a directory. Batch mode is
// non-interactive by design (no per-file confirmation prompt -- treated as -y);
// it collects a structured result per file and prints either a summary table
// or, with --json, a JSON array.

// ErrBatchFailures signals that a batch ran but at least one file failed. The
// CLI maps it to exit code 1 WITHOUT printing an extra error line, because the
// per-file errors are already in the table / JSON output.
var ErrBatchFailures = errors.New("batch completed with failures")

// BatchResult is one file's outcome. Field tags define the --json schema.
type BatchResult struct {
	Input          string `json:"input"`
	Output         string `json:"output"`
	OK             bool   `json:"ok"`
	DetectedFormat string `json:"detected_format,omitempty"`
	PayloadStatus  string `json:"payload_status,omitempty"` // MATCH / MISMATCH / UNVERIFIED
	Structure      string `json:"structure,omitempty"`      // PASS / FAIL / SKIP (Step 6)
	Error          string `json:"error,omitempty"`
}

// Dispatch routes a run: a directory input goes to RunBatch, anything else
// (a file, or a missing path that Run will report) to the single-file Run.
func Dispatch(opts RunOptions, s IO) error {
	if info, err := os.Stat(opts.InputFile); err == nil && info.IsDir() {
		return RunBatch(opts, s)
	}
	return Run(opts, s)
}

// collectFiles returns the regular files to process under dir. Non-recursive by
// default (top level only); Recursive walks the whole tree. The list is
// gathered up front so output files created during the run are never picked up
// as new inputs. Names are sorted for deterministic output.
func collectFiles(dir string, recursive bool) ([]string, error) {
	var files []string
	if recursive {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Type().IsRegular() {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// batchExt is the output extension for the whole batch: the preset's natural
// extension, or "mbyte" for a raw --hex run (mirrors ResolveOutput's default).
func batchExt(opts RunOptions) string {
	if opts.Preset != "" {
		if e, ok := PresetExtension(opts.Preset); ok {
			return e
		}
	}
	return "mbyte"
}

// processFile applies the operation to one input and returns its result. It
// never prompts and never panics; any failure is captured in Result.Error.
// When dryRun is true the output bytes are computed but not written.
func processFile(opts RunOptions, magic []byte, inputFile, output string, dryRun bool) BatchResult {
	res := BatchResult{Input: inputFile, Output: output}

	original, err := readInput(inputFile)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	// Reuse the single-file preview builder for the payload-hash comparison.
	one := RunOptions{Command: opts.Command, InputFile: inputFile, Raw: opts.Raw}
	p, err := buildPreview(one, output, magic, original)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if !dryRun {
		if err := os.WriteFile(output, p.outData, 0o644); err != nil {
			res.Error = err.Error()
			return res
		}
	}
	head := p.outData
	if len(head) > 16 {
		head = head[:16]
	}
	res.OK = true
	res.DetectedFormat = DetectFormat(head)
	res.PayloadStatus = p.statusCode
	// Step 6: structural check of the full output (unless --no-check).
	if !opts.NoCheck {
		status, _, _ := CheckBytes(p.outData)
		res.Structure = status.String()
	}
	return res
}

// RunBatch applies the operation to every file under opts.InputFile (a dir).
func RunBatch(opts RunOptions, s IO) error {
	files, err := collectFiles(opts.InputFile, opts.Recursive)
	if err != nil {
		return &MbyteError{Msg: fmt.Sprintf("cannot read input directory %s: %v", opts.InputFile, err)}
	}

	// Resolve the magic bytes once; the same operation applies to every file.
	hexBytes, err := ResolveHex(opts.Preset, opts.Hex)
	if err != nil {
		return err
	}
	magic, err := ParseHex(hexBytes)
	if err != nil {
		return err
	}

	// Output directory: -o if given (created if missing), else the input dir.
	outDir := opts.Output
	if outDir == "" {
		outDir = opts.InputFile
	}
	if !opts.DryRun {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return &MbyteError{Msg: fmt.Sprintf("cannot create output directory %s: %v", outDir, err)}
		}
	}

	ext := batchExt(opts)
	results := make([]BatchResult, 0, len(files))
	for _, f := range files {
		out := filepath.Join(outDir, filepath.Base(f)+"."+ext)
		results = append(results, processFile(opts, magic, f, out, opts.DryRun))
	}

	if opts.JSON {
		if err := writeBatchJSON(s.Out, results); err != nil {
			return err
		}
	} else {
		writeBatchTable(s.Out, results, opts.DryRun)
	}

	for _, r := range results {
		if !r.OK {
			return ErrBatchFailures
		}
	}
	return nil
}

// writeBatchJSON prints results as an indented JSON array.
func writeBatchJSON(w io.Writer, results []BatchResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// writeBatchTable prints a human-readable summary with aligned columns.
func writeBatchTable(w io.Writer, results []BatchResult, dryRun bool) {
	okN := 0
	for _, r := range results {
		if r.OK {
			okN++
		}
	}
	verb := "processed"
	if dryRun {
		verb = "would process (dry-run)"
	}
	fmt.Fprintf(w, "[batch] %s %d file(s): %d ok, %d failed\n\n", verb, len(results), okN, len(results)-okN)
	if len(results) == 0 {
		fmt.Fprintln(w, "  (no files found)")
		return
	}

	// Column widths.
	hStatus, hInput, hFormat, hPayload, hStruct := "STATUS", "INPUT", "FORMAT", "PAYLOAD", "STRUCT"
	wStatus, wInput, wFormat, wPayload, wStruct := len(hStatus), len(hInput), len(hFormat), len(hPayload), len(hStruct)
	statusOf := func(r BatchResult) string {
		if r.OK {
			return "ok"
		}
		return "FAIL"
	}
	for _, r := range results {
		wStatus = maxInt(wStatus, len(statusOf(r)))
		wInput = maxInt(wInput, len(filepath.Base(r.Input)))
		wFormat = maxInt(wFormat, len(r.DetectedFormat))
		wPayload = maxInt(wPayload, len(r.PayloadStatus))
		wStruct = maxInt(wStruct, len(r.Structure))
	}

	fmt.Fprintf(w, "  %s  %s  %s  %s  %s  %s\n",
		pad(hStatus, wStatus), pad(hInput, wInput), pad(hFormat, wFormat),
		pad(hPayload, wPayload), pad(hStruct, wStruct), "OUTPUT / ERROR")
	for _, r := range results {
		last := r.Output
		if !r.OK {
			last = "error: " + r.Error
		}
		fmt.Fprintf(w, "  %s  %s  %s  %s  %s  %s\n",
			pad(statusOf(r), wStatus),
			pad(filepath.Base(r.Input), wInput),
			pad(r.DetectedFormat, wFormat),
			pad(r.PayloadStatus, wPayload),
			pad(r.Structure, wStruct),
			last)
	}
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
