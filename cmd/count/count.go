package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// run prints to out (and errors to errOut) all lines read from r that end
// with " 0". It returns a non-nil error only when reading fails.
func run(r io.Reader, out, errOut io.Writer) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, " 0") {
			fmt.Fprintln(out, line)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(errOut, err)
		return err
	}
	return nil
}

// Main is the entry point for the count command. It returns the process exit
// code (0 = success, 1 = runtime error, 2 = usage error) and writes any
// diagnostics to errOut. It never calls os.Exit, so tests can drive it
// in-process.
func Main(args []string, out, errOut io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(errOut, "usage: count <file>")
		return 2
	}
	f, err := os.Open(args[1])
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	defer f.Close()
	if err := run(f, out, errOut); err != nil {
		return 1
	}
	return 0
}

// exitFn is the function used by main to terminate the process. It is
// overridable from tests so the binary entry point can be exercised without
// killing the test binary.
var exitFn = os.Exit

func main() {
	exitFn(Main(os.Args, os.Stdout, os.Stderr))
}
