// Command mbyte is the CLI entry point. It is a thin cobra wrapper: it wires
// flags into mbyte.RunOptions and maps errors to exit codes, mirroring the
// structure of mbyte/cli.py. All behavior lives in the internal/mbyte package.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Akira0412-degitech/Mbyte/internal/mbyte"
	"github.com/spf13/cobra"
)

const version = "0.1.0"

// errNoCommand signals "no subcommand given" -> print help and exit 1,
// matching cli.py main() (command is None -> print_help, return 1).
var errNoCommand = errors.New("no command")

func main() {
	os.Exit(runCLI())
}

func runCLI() int {
	root := newRootCmd()
	err := root.Execute()
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, errNoCommand):
		// help was already printed by the root RunE
		return 1
	case errors.Is(err, mbyte.ErrBatchFailures):
		// batch ran but some files failed; the per-file errors are already in
		// the table/JSON output, so just signal failure via the exit code.
		return 1
	case isRuntimeError(err):
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	default:
		// usage / flag errors (unknown flag, missing or conflicting
		// --preset/--hex, wrong arg count) -> argparse-style exit code 2
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
}

// isRuntimeError reports whether err is a core runtime failure (MbyteError or a
// missing input file) as opposed to a cobra usage/flag error.
func isRuntimeError(err error) bool {
	var me *mbyte.MbyteError
	return errors.As(err, &me) || errors.Is(err, os.ErrNotExist)
}

// isTerminal reports whether f is attached to an interactive terminal, using
// only the standard library (no x/term dependency): a character device is a
// TTY, while a pipe or redirected file is not.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func newRootCmd() *cobra.Command {
	var listPresets bool

	root := &cobra.Command{
		Use:   "mbyte",
		Short: "Insert or replace a file's leading magic bytes. For CTF / authorized-testing education only.",
		// We print errors and usage ourselves (see runCLI) to control exit codes.
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listPresets {
				fmt.Fprintln(cmd.OutOrStdout(), mbyte.ListPresets())
				return nil
			}
			_ = cmd.Help()
			return errNoCommand
		},
	}
	root.SetVersionTemplate("mbyte {{.Version}}\n")
	root.Flags().BoolVar(&listPresets, "list-presets", false,
		"list available magic-byte presets and exit")

	root.AddCommand(newActionCmd("insert", "prepend magic bytes (keeps original content)", false))
	root.AddCommand(newActionCmd("replace", "overwrite the first N bytes with magic bytes", true))
	return root
}

// newActionCmd builds the insert or replace subcommand. withRaw adds the
// replace-only --raw flag.
func newActionCmd(name, short string, withRaw bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:           name,
		Short:         short,
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := mbyte.RunOptions{Command: name, InputFile: args[0]}
			opts.Output, _ = cmd.Flags().GetString("output")
			opts.Preset, _ = cmd.Flags().GetString("preset")
			opts.Hex, _ = cmd.Flags().GetString("hex")
			opts.Force, _ = cmd.Flags().GetBool("force")
			opts.Verbose, _ = cmd.Flags().GetBool("verbose")
			opts.Yes, _ = cmd.Flags().GetBool("yes")
			opts.DryRun, _ = cmd.Flags().GetBool("dry-run")
			opts.Recursive, _ = cmd.Flags().GetBool("recursive")
			opts.JSON, _ = cmd.Flags().GetBool("json")
			if withRaw {
				opts.Raw, _ = cmd.Flags().GetBool("raw")
			}
			streams := mbyte.IO{
				Out: cmd.OutOrStdout(),
				Err: cmd.ErrOrStderr(),
				In:  cmd.InOrStdin(),
				// Only prompt when stdin is a real terminal; piped/scripted
				// (non-TTY) runs proceed without asking, so existing non-
				// interactive usage keeps working unchanged.
				Interactive: isTerminal(os.Stdin),
			}
			return mbyte.Dispatch(opts, streams)
		},
	}

	cmd.Flags().StringP("output", "o", "", "output file path (default: <input_file>.mbyte)")
	cmd.Flags().String("preset", "", "named magic-byte preset (see --list-presets)")
	cmd.Flags().String("hex", "", "raw hex string, e.g. 89504e47")
	cmd.Flags().Bool("force", false, "allow writing over the input file (disabled by default)")
	cmd.Flags().BoolP("verbose", "v", false, "print detailed logs")
	cmd.Flags().BoolP("yes", "y", false, "skip the confirmation prompt (assume yes)")
	cmd.Flags().Bool("dry-run", false, "show the preview and exit without writing")
	cmd.Flags().BoolP("recursive", "r", false, "when input is a directory, recurse into subdirectories")
	cmd.Flags().Bool("json", false, "when input is a directory, print results as a JSON array")
	if withRaw {
		cmd.Flags().Bool("raw", false,
			"skip signature-boundary detection; overwrite exactly len(hex) bytes from the start "+
				"(old/unsafe behavior -- may corrupt data or leave garbage bytes if the length "+
				"doesn't match the file's real signature)")
	}

	// --preset and --hex: exactly one is required (argparse's required mutually
	// exclusive group).
	cmd.MarkFlagsMutuallyExclusive("preset", "hex")
	cmd.MarkFlagsOneRequired("preset", "hex")
	return cmd
}
