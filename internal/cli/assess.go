package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/timothymamo/eu-sov-cloud/internal/app/analysis"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// assessOptions are the flags of the assess command.
type assessOptions struct {
	// inventory is the path of the inventory file.
	inventory string
	// target is the key of the target in Deps.Targets.
	target string
	// output is the path of the report file, or empty for standard output.
	output string
}

// runAssess runs the assess command with args, the arguments after "assess",
// and returns the exit code.
func runAssess(ctx context.Context, args []string, stdout, stderr io.Writer, deps Deps) int {
	opts, err := parseAssess(args, deps)

	switch {
	case errors.Is(err, flag.ErrHelp):
		printUsage(stdout, deps)

		return exitOK
	case err != nil:
		return usageError(stderr, deps, err)
	}

	if err := assess(ctx, opts, stdout, deps); err != nil {
		return runError(stderr, err)
	}

	return exitOK
}

// parseAssess parses the flags of the assess command and checks them against
// deps. Every error it returns is a usage error, or flag.ErrHelp when help was
// asked for.
func parseAssess(args []string, deps Deps) (assessOptions, error) {
	var opts assessOptions

	flags := flag.NewFlagSet("assess", flag.ContinueOnError)
	// The flag package's own messages show the command line unescaped, so it
	// prints nothing: its errors reach the user through printError.
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	flags.StringVar(&opts.inventory, "inventory", "", "the inventory file")
	flags.StringVar(&opts.target, "target", "", "the target to compare with")
	flags.StringVar(&opts.output, "output", "", "the report file")

	if err := flags.Parse(args); err != nil {
		return assessOptions{}, err
	}

	switch {
	case opts.inventory == "":
		return assessOptions{}, errors.New("assess needs --inventory")
	case opts.target == "":
		return assessOptions{}, errors.New("assess needs --target")
	case flags.NArg() > 0:
		return assessOptions{}, fmt.Errorf("unexpected argument %s", text.Quote(flags.Arg(0)))
	}

	if _, ok := deps.Targets[opts.target]; !ok {
		return assessOptions{}, fmt.Errorf("unknown target %s; supported targets: %s",
			text.Quote(opts.target), strings.Join(targetKeys(deps), ", "))
	}

	return opts, nil
}

// assess builds the adapters from deps, runs the analysis and writes the report
// to stdout or to the --output file. It refuses an unsafe --output before it
// reads anything, renders the whole report into memory, and writes nothing
// unless the whole report was rendered.
func assess(ctx context.Context, opts assessOptions, stdout io.Writer, deps Deps) error {
	newCatalog := deps.Targets[opts.target]
	if deps.NewInventorySource == nil || newCatalog == nil || deps.NewReporter == nil {
		return errMissingAdapter
	}

	run, err := analysis.New(deps.NewInventorySource(opts.inventory), newCatalog(), deps.NewReporter())
	if err != nil {
		return err
	}

	if opts.output != "" {
		if err := checkOutput(opts.inventory, opts.output); err != nil {
			return err
		}
	}

	// A report names the inventory file by its base name, never by its full
	// path; analysis.Run checks the name before it reads anything.
	meta := analysis.ReportMeta{InventoryName: filepath.Base(opts.inventory)}

	var report bytes.Buffer

	if err := run.Run(ctx, &report, meta); err != nil {
		return err
	}

	// A run cancelled while the report was rendered writes nothing: the user
	// asked it to stop.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelled before writing the report: %w", err)
	}

	if opts.output == "" {
		return writeStdout(stdout, report.Bytes())
	}

	return writeFile(opts.output, report.Bytes())
}

// checkOutput refuses an --output dest the report must not replace: anything
// but a regular file or a symbolic link (a directory, a named pipe, a socket, a
// device), and the inventory file itself, by the same path, a hard link or a
// symbolic link to it. A dest that does not exist yet is accepted.
func checkOutput(inventory, dest string) error {
	info, err := os.Lstat(dest)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("output %s: %w", text.Quote(dest), withoutPath(err))
	case !info.Mode().IsRegular() && info.Mode()&fs.ModeSymlink == 0:
		return fmt.Errorf("output %s is %s, not a regular file", text.Quote(dest), fileKind(info.Mode()))
	}

	// os.Stat follows symbolic links on both sides, so a link to the inventory,
	// or an inventory that is a link to dest, is the same file. A path that
	// cannot be resolved is no clash: the inventory source reports a missing
	// inventory, and a dangling link at dest is replaced.
	invInfo, invErr := os.Stat(inventory)
	destInfo, destErr := os.Stat(dest)

	if invErr == nil && destErr == nil && os.SameFile(invInfo, destInfo) {
		return fmt.Errorf("output %s is the inventory file %s; write the report to another file",
			text.Quote(dest), text.Quote(inventory))
	}

	return nil
}

// fileKind names the kind of file mode describes, for an output that is not a
// regular file.
func fileKind(mode fs.FileMode) string {
	switch {
	case mode.IsDir():
		return "a directory"
	case mode&fs.ModeNamedPipe != 0:
		return "a named pipe"
	case mode&fs.ModeSocket != 0:
		return "a socket"
	case mode&fs.ModeDevice != 0:
		return "a device"
	default:
		return "a special file"
	}
}

// writeStdout writes the rendered report to stdout in one call.
func writeStdout(stdout io.Writer, report []byte) error {
	if _, err := stdout.Write(report); err != nil {
		return fmt.Errorf("write the report to standard output: %w", withoutPath(err))
	}

	return nil
}

// reportMode is the mode of a report file: a report describes an account's
// infrastructure, so only its owner may read it.
const reportMode = 0o600

// writeFile writes the rendered report to a temporary file in the directory of
// dest and renames it onto dest once it is written and synced. The temporary
// file exists only for this final write, never while the report is rendered.
// On any error it removes the temporary file, so dest is either the whole new
// report or left as it was, and never truncated. Every error names dest, never
// the temporary file.
func writeFile(dest string, report []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".eusov-report-*.tmp")
	if err != nil {
		return fmt.Errorf("output %s: create a temporary file in its directory: %w",
			text.Quote(dest), withoutPath(err))
	}

	renamed := false

	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if err := tmp.Chmod(reportMode); err != nil {
		return fmt.Errorf("output %s: set the mode of the temporary file: %w", text.Quote(dest), withoutPath(err))
	}

	if _, err := tmp.Write(report); err != nil {
		return fmt.Errorf("output %s: write the report: %w", text.Quote(dest), withoutPath(err))
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("output %s: sync the temporary file: %w", text.Quote(dest), withoutPath(err))
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("output %s: close the temporary file: %w", text.Quote(dest), withoutPath(err))
	}

	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("output %s: replace it with the report: %w", text.Quote(dest), withoutPath(err))
	}

	renamed = true

	return nil
}

// withoutPath returns the OS error inside an *fs.PathError or *os.LinkError,
// which repeat file names unquoted, so that an error names the file once,
// through text.Quote. Any other error is returned as it is.
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}

	return err
}
