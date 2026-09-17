package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirResolution(t *testing.T) {
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", "/custom/dir")
	if DataDir() != "/custom/dir" {
		t.Errorf("env override failed: %s", DataDir())
	}

	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/xdg")
	if DataDir() != "/xdg/antigravity-seo" {
		t.Errorf("XDG resolution failed: %s", DataDir())
	}
}

func TestDataDirPerOS(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	toPortable := func(p string) string { return strings.ReplaceAll(p, "\\", "/") }

	if got := toPortable(dataDirFor("darwin")); got != "/home/tester/Library/Application Support/antigravity-seo" {
		t.Errorf("darwin data dir wrong: %q", got)
	}

	t.Setenv("LOCALAPPDATA", `C:\Users\tester\AppData\Local`)
	if got := toPortable(dataDirFor("windows")); got != "C:/Users/tester/AppData/Local/antigravity-seo" {
		t.Errorf("windows data dir (LOCALAPPDATA) wrong: %q", got)
	}

	t.Setenv("LOCALAPPDATA", "")
	if got := toPortable(dataDirFor("windows")); got != "/home/tester/AppData/Local/antigravity-seo" {
		t.Errorf("windows data dir (fallback) wrong: %q", got)
	}

	t.Setenv("XDG_DATA_HOME", "/xdg")
	if got := toPortable(dataDirFor("linux")); got != "/xdg/antigravity-seo" {
		t.Errorf("linux data dir wrong: %q", got)
	}
}

func TestSetupAndDoctor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ANTIGRAVITY_SEO_DATA_DIR", dir)

	// Doctor before setup must be not ready
	pre := RunDoctor("2.0.0", true)
	if pre.Ready {
		t.Fatal("doctor should not be ready before setup")
	}

	setupReport, err := RunSetup("2.0.0", ".")
	if err != nil {
		t.Fatal(err)
	}
	if setupReport.DataDir != dir {
		t.Errorf("setup data dir mismatch: %s", setupReport.DataDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "drift")); err != nil {
		t.Error("drift dir not created")
	}
	if _, err := os.Stat(StatePath()); err != nil {
		t.Error("runtime-state.json not written")
	}

	post := RunDoctor("2.0.0", true)
	if !post.Ready {
		t.Fatalf("doctor should be ready after setup: %+v", post.Checks)
	}

	// Version drift should downgrade to warn but keep essential readiness
	post2 := RunDoctor("9.9.9", true)
	stateCheck := findCheck(post2, "runtime_state")
	if stateCheck == nil || stateCheck.Status != StatusWarn {
		t.Errorf("stale state should warn, got %+v", stateCheck)
	}
}

func TestDetectIntegrations(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("INDEXNOW_KEY", "")
	ints := DetectIntegrations()
	if ints.GoogleAPIKey || ints.IndexNowKey {
		t.Errorf("empty env should disable integrations: %+v", ints)
	}

	t.Setenv("GOOGLE_API_KEY", "x")
	ints = DetectIntegrations()
	if !ints.GoogleAPIKey || ints.IndexNowKey {
		t.Errorf("only google key should be set: %+v", ints)
	}
}

func findCheck(r *DoctorReport, name string) *Check {
	for i := range r.Checks {
		if r.Checks[i].Name == name {
			return &r.Checks[i]
		}
	}
	return nil
}
