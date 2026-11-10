// Package cli is the driving adapter: it parses the command line, calls the
// analysis use case and prints the result. It contains no business rules and
// constructs no adapter: the composition root (cmd/eusov) passes the adapters
// in through Deps, and Run builds them only once the command line is valid.
//
// # Commands
//
//	eusov assess --inventory <file> --target <target> [--output <file>]
//	eusov version
//	eusov help
//
// assess reads the inventory file, loads the catalog of the target, assesses
// every resource and writes the report. --inventory is a file path: standard
// input is not supported, so a source is always a file the inventory-file
// adapter reads once per run (ADR 0006). The report names the file by its base
// name only (analysis.ReportMeta). Without --output the report goes to standard
// output; with it, to the named file. Flags are parsed by the standard library
// flag package, so -flag and --flag are the same.
//
// # Exit codes
//
//   - 0: the command ran. For assess, blockers are a result, not a failure.
//     Only the command's output is written, to standard output or to the
//     --output file; nothing goes to standard error.
//   - 1: a run-time error: the inventory, the catalog, the report or the output
//     failed, or the run was cancelled (SIGINT, SIGTERM or SIGHUP cancel the
//     context in cmd/eusov). Exactly one error line goes to standard error and
//     nothing to standard output.
//   - 2: a usage error: no command or an unknown one, a missing --inventory or
//     --target, an unknown target (the error lists the supported ones), an
//     unknown flag or an extra argument. An error line and the usage text go to
//     standard error, nothing goes to standard output, and no adapter is built.
//
// Help (help, -h, --help, or -h after assess) prints the usage text to standard
// output and exits 0.
//
// # Errors
//
// Every error, run-time or usage, is printed by printError as one line,
// "eusov: " and the message passed through sanitise. sanitise escapes control,
// format (Cf), line separator (Zl) and paragraph separator (Zp) characters and
// invalid UTF-8 bytes as strconv does (\n, \x1b, \u202e), so a message never
// breaks the line, moves the cursor or reorders the text, whatever the input
// file, the OS or an adapter put in it. The escaped message is capped at 1 KiB,
// then a truncation marker; the cut falls between characters, never inside an
// escape. The flag package prints nothing itself:
// its errors also go through printError.
//
// # Writing the report
//
// The report is rendered into memory in full before anything is written, so a
// failed run leaves no partial report anywhere. Without --output it is written
// to standard output once complete. With --output it is written to a temporary
// file created in the destination's directory only after rendering, set to
// mode 0600, synced and closed, then renamed onto the destination. On any error
// the temporary file is removed and an existing destination is left untouched,
// never truncated. A run cancelled before the report is written writes nothing.
//
// What --output may name is checked before the inventory is read:
//
//   - a path that does not exist yet: the report is created there;
//   - an existing regular file: it is replaced atomically, in one rename;
//   - a symbolic link: the link itself is replaced, never followed;
//   - anything else (a directory, a named pipe, a socket, a device): refused
//     with exit code 1;
//   - the inventory file itself, by the same path, a hard link or a symbolic
//     link to it: refused with exit code 1, so a report never replaces the
//     inventory it assesses.
//
// A refused destination is left as it was and no temporary file is created.
package cli
