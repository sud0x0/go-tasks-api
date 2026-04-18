package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestCurrent(t *testing.T) {
	info := Current()

	// GoVersion must match runtime.Version()
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}

	// OSArch must match runtime.GOOS + "/" + runtime.GOARCH
	expectedOSArch := runtime.GOOS + "/" + runtime.GOARCH
	if info.OSArch != expectedOSArch {
		t.Errorf("OSArch = %q, want %q", info.OSArch, expectedOSArch)
	}

	// Version must be non-empty (either ldflags value or "dev" fallback)
	if info.Version == "" {
		t.Error("Version is empty")
	}

	// GitCommit must be non-empty (either ldflags value or "unknown" fallback)
	if info.GitCommit == "" {
		t.Error("GitCommit is empty")
	}

	// BuildDate must be non-empty (either ldflags value or "unknown" fallback)
	if info.BuildDate == "" {
		t.Error("BuildDate is empty")
	}
}

func TestBanner(t *testing.T) {
	t.Run("format with go-tasks-api", func(t *testing.T) {
		info := Current()
		banner := info.Banner("go-tasks-api")

		// Must start with app name and version
		if !strings.HasPrefix(banner, "go-tasks-api version ") {
			t.Errorf("Banner does not start with 'go-tasks-api version ', got:\n%s", banner)
		}

		// Must contain expected labels with exact spacing
		expectedLabels := []string{
			"Git commit: ",
			"Build date: ",
			"Go version: ",
			"OS/Arch:    ",
		}
		for _, label := range expectedLabels {
			if !strings.Contains(banner, label) {
				t.Errorf("Banner missing label %q, got:\n%s", label, banner)
			}
		}

		// Must end with newline
		if !strings.HasSuffix(banner, "\n") {
			t.Errorf("Banner does not end with newline, got:\n%q", banner)
		}

		// Must have exactly 5 lines
		lines := strings.Split(strings.TrimSuffix(banner, "\n"), "\n")
		if len(lines) != 5 {
			t.Errorf("Banner has %d lines, want 5, got:\n%s", len(lines), banner)
		}
	})

	t.Run("custom app name", func(t *testing.T) {
		info := Current()
		banner := info.Banner("my-custom-app")

		if !strings.HasPrefix(banner, "my-custom-app version ") {
			t.Errorf("Banner does not start with 'my-custom-app version ', got:\n%s", banner)
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		info := Info{
			Version:   "1.0.0",
			GitCommit: "abc1234",
			BuildDate: "2026-01-01T00:00:00Z",
			GoVersion: "go1.26",
			OSArch:    "linux/amd64",
		}

		banner1 := info.Banner("test-app")
		banner2 := info.Banner("test-app")

		if banner1 != banner2 {
			t.Errorf("Banner is not deterministic:\nFirst:  %q\nSecond: %q", banner1, banner2)
		}
	})
}
