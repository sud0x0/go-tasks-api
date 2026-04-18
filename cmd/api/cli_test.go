package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleFlags(t *testing.T) {
	t.Run("--version flag", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{"--version"}, &buf)

		if !shouldExit {
			t.Error("expected shouldExit=true for --version")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}
		if !strings.Contains(buf.String(), "go-tasks-api version") {
			t.Errorf("expected banner in output, got:\n%s", buf.String())
		}
	})

	t.Run("-v flag", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{"-v"}, &buf)

		if !shouldExit {
			t.Error("expected shouldExit=true for -v")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}
		if !strings.Contains(buf.String(), "go-tasks-api version") {
			t.Errorf("expected banner in output, got:\n%s", buf.String())
		}
	})

	t.Run("no flags", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{}, &buf)

		if shouldExit {
			t.Error("expected shouldExit=false for no flags")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}
		if buf.Len() != 0 {
			t.Errorf("expected empty output, got:\n%s", buf.String())
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{"--frobnicate"}, &buf)

		if shouldExit {
			t.Error("expected shouldExit=false for unknown flag")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}
		if buf.Len() != 0 {
			t.Errorf("expected empty output, got:\n%s", buf.String())
		}
	})

	t.Run("--version --version", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{"--version", "--version"}, &buf)

		if !shouldExit {
			t.Error("expected shouldExit=true for --version --version")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}

		// Banner should appear exactly once
		output := buf.String()
		count := strings.Count(output, "go-tasks-api version")
		if count != 1 {
			t.Errorf("expected banner once, got %d times in:\n%s", count, output)
		}
	})

	t.Run("-version flag (stdlib accepts single dash)", func(t *testing.T) {
		var buf bytes.Buffer
		shouldExit, code := handleFlags([]string{"-version"}, &buf)

		if !shouldExit {
			t.Error("expected shouldExit=true for -version")
		}
		if code != 0 {
			t.Errorf("expected exitCode=0, got %d", code)
		}
		if !strings.Contains(buf.String(), "go-tasks-api version") {
			t.Errorf("expected banner in output, got:\n%s", buf.String())
		}
	})
}
