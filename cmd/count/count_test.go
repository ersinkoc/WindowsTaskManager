package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// errReader returns a non-EOF error on Read to force bufio.Scanner.Err()
// to be non-nil and exercise both the error-print and return-error paths
// in run().
type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("boom") }

func TestRun_PrintsOnlyLinesEndingWithSpaceZero(t *testing.T) {
	input := "alpha 0\n" +
		"beta 1\n" +
		"gamma 0\n" +
		"delta 9\n" +
		"epsilon 0\n"

	var out, errOut bytes.Buffer
	if err := run(strings.NewReader(input), &out, &errOut); err != nil {
		t.Fatalf("run returned unexpected error: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}

	got := out.String()
	want := "alpha 0\n" + "gamma 0\n" + "epsilon 0\n"
	if got != want {
		t.Fatalf("stdout mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestRun_NoMatchingLines(t *testing.T) {
	input := "no match here\nstill no match\n"
	var out, errOut bytes.Buffer
	if err := run(strings.NewReader(input), &out, &errOut); err != nil {
		t.Fatalf("run returned unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOut.String())
	}
}

func TestRun_EmptyInput(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run(strings.NewReader(""), &out, &errOut); err != nil {
		t.Fatalf("run returned unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errOut.String())
	}
}

func TestRun_ScannerErrorIsReported(t *testing.T) {
	var out, errOut bytes.Buffer
	err := run(errReader{}, &out, &errOut)
	if err == nil {
		t.Fatal("expected run to return an error, got nil")
	}
	if !strings.Contains(errOut.String(), "boom") {
		t.Fatalf("expected stderr to contain the scanner error, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout on error, got %q", out.String())
	}
}

func TestRealMain_WithTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.txt")

	contents := "keep me 0\n" +
		"skip me 1\n" +
		"keep me too 0\n" +
		"trailing space 0 \n" // " 0 " is NOT a " 0" suffix

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var out, errOut bytes.Buffer
	if code := Main([]string{"count", path}, &out, &errOut); code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}

	want := "keep me 0\n" + "keep me too 0\n"
	if got := out.String(); got != want {
		t.Fatalf("stdout mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestRealMain_MissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")

	var out, errOut bytes.Buffer
	code := Main([]string{"count", missing}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1 for missing file, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout on error, got %q", out.String())
	}
	// Windows: "The system cannot find the file specified."
	// Unix:    "no such file or directory"
	if !strings.Contains(errOut.String(), "no such file") &&
		!strings.Contains(errOut.String(), "cannot find") &&
		!strings.Contains(errOut.String(), "does-not-exist.txt") {
		t.Fatalf("expected stderr to mention missing file, got %q", errOut.String())
	}
}

func TestRealMain_MissingArgument(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Main([]string{"count"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2 for missing argument, got %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("expected stderr to contain usage hint, got %q", errOut.String())
	}
}

// TestMain_ScannerErrorFromFile covers the branch inside Main where run()
// returns a non-nil error. bufio.Scanner reports "token too long" when a
// line exceeds its default buffer; we trigger that by writing a single
// line longer than 64 KiB.
func TestMain_ScannerErrorFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	hugeLine := strings.Repeat("x", 200_000)
	if err := os.WriteFile(path, []byte(hugeLine), 0o644); err != nil {
		t.Fatalf("write huge file: %v", err)
	}

	var out, errOut bytes.Buffer
	code := Main([]string{"count", path}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1 when scanner fails, got %d (stderr: %s)", code, errOut.String())
	}
	if errOut.Len() == 0 {
		t.Fatal("expected stderr to contain scanner error, got empty")
	}
}

// TestMainEntry_PassesExitCodeThrough exercises func main() by swapping the
// exitFn hook for a recorder and redirecting os.Stdout/os.Stderr/os.Args.
// This is the only way to put main() into the coverage profile under
// `go test`, since `go test` never executes the binary's package main
// entry point on its own.
func TestMainEntry_PassesExitCodeThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("hit 0\nmiss 1\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	origArgs := os.Args
	origStdout := os.Stdout
	origStderr := os.Stderr
	origExit := exitFn
	defer func() {
		os.Args = origArgs
		os.Stdout = origStdout
		os.Stderr = origStderr
		exitFn = origExit
	}()

	os.Args = []string{"count", path}
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW
	exitFn = func(_ int) {}

	main()

	stdoutW.Close()
	stderrW.Close()
	var outBuf, errBuf bytes.Buffer
	outBuf.ReadFrom(stdoutR)
	errBuf.ReadFrom(stderrR)

	if !strings.Contains(outBuf.String(), "hit 0") {
		t.Fatalf("expected stdout to contain 'hit 0', got %q", outBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", errBuf.String())
	}
}

func TestMainEntry_MissingFileExitsNonZero(t *testing.T) {
	origArgs := os.Args
	origStdout := os.Stdout
	origStderr := os.Stderr
	origExit := exitFn
	defer func() {
		os.Args = origArgs
		os.Stdout = origStdout
		os.Stderr = origStderr
		exitFn = origExit
	}()

	os.Args = []string{"count", filepath.Join(t.TempDir(), "nope.txt")}
	_, w1, _ := os.Pipe()
	_, w2, _ := os.Pipe()
	os.Stdout = w1
	os.Stderr = w2
	defer w1.Close()
	defer w2.Close()

	var captured int
	exitFn = func(code int) { captured = code }
	main()

	if captured != 1 {
		t.Fatalf("expected captured exit code 1, got %d", captured)
	}
}
