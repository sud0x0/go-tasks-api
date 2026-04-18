package main

import (
	"flag"
	"fmt"
	"io"

	"go-tasks-api/internal/version"
)

// handleFlags parses CLI flags that should short-circuit startup.
// It is extracted for testability — takes args and an output writer so
// tests can drive it without touching os.Args or os.Stdout.
// Returns (shouldExit, exitCode). If shouldExit is false, main() continues.
//
//nolint:unparam // exitCode is always 0 today; signature allows future flags to return non-zero
func handleFlags(args []string, out io.Writer) (bool, int) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	// Suppress default usage output; we do not advertise a CLI surface
	// beyond --version for this binary.
	fs.SetOutput(io.Discard)

	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "print version information and exit")
	fs.BoolVar(&showVersion, "v", false, "print version information and exit (shorthand)")

	if err := fs.Parse(args); err != nil {
		// Unknown or malformed flags: ignore silently and fall through
		// to normal startup. This preserves the previous behaviour of
		// the binary taking no flags.
		return false, 0
	}

	if showVersion {
		_, _ = fmt.Fprint(out, version.Current().Banner("go-tasks-api"))
		return true, 0
	}

	return false, 0
}
