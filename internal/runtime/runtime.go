// Package runtime provides setup and doctor infrastructure for the engine.
// It mirrors the operational contract of an installable SEO plugin: a
// persistent data directory, a runtime-state manifest, and a readiness check.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// RuntimeSchemaVersion guards runtime-state.json format migrations
const RuntimeSchemaVersion = 1

// StateFileName is the manifest written into the data dir by setup
const StateFileName = "runtime-state.json"

// CheckStatus classifies a doctor check outcome
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
	StatusInfo CheckStatus = "INFO"
)

// Check is a single doctor readiness probe
type Check struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

// Integrations reports which optional API integrations have credentials
type Integrations struct {
	GoogleAPIKey bool   `json:"google_api_key"`
	IndexNowKey  bool   `json:"indexnow_key"`
	GSCAuth      bool   `json:"gsc_auth"`
	PDFRenderer  string `json:"pdf_renderer,omitempty"`
}

// RuntimeState is the persisted manifest describing the installed runtime
type RuntimeState struct {
	RuntimeSchema int          `json:"runtime_schema"`
	EngineVersion string       `json:"engine_version"`
	DataDir       string       `json:"data_dir"`
	Adapters      []string     `json:"adapters"`
	Integrations  Integrations `json:"integrations"`
	CreatedAt     string       `json:"created_at"`
}

// SetupReport summarizes what setup did
type SetupReport struct {
	DataDir      string     `json:"data_dir"`
	StatePath    string     `json:"state_path"`
	DriftDir     string     `json:"drift_dir"`
	Adapters     []string   `json:"adapters"`
	Integrations Integrations `json:"integrations"`
	Warnings     []string   `json:"warnings,omitempty"`
}

// DoctorReport is the full readiness assessment
type DoctorReport struct {
	Ready         bool         `json:"ready"`
	EngineVersion string       `json:"engine_version"`
	Platform      string       `json:"platform"` // GOOS/GOARCH
	DataDir       string       `json:"data_dir"`
	StatePresent  bool         `json:"runtime_state_present"`
	Checks        []Check      `json:"checks"`
	Integrations  Integrations `json:"integrations"`
}

// DataDir resolves the persistent data directory:
// ANTIGRAVITY_SEO_DATA_DIR > OS-specific default:
//   Linux/other: XDG_DATA_HOME/antigravity-seo or ~/.local/share/antigravity-seo
//   macOS:       ~/Library/Application Support/antigravity-seo
//   Windows:     %LOCALAPPDATA%\antigravity-seo
func DataDir() string {
	if d := os.Getenv("ANTIGRAVITY_SEO_DATA_DIR"); d != "" {
		return d
	}
	return dataDirFor(runtime.GOOS)
}

// dataDirFor returns the per-OS default; split from DataDir for testability
func dataDirFor(goos string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "antigravity-seo")
	}
	switch goos {
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "antigravity-seo")
		}
		return filepath.Join(home, "AppData", "Local", "antigravity-seo")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "antigravity-seo")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "antigravity-seo")
		}
		return filepath.Join(home, ".local", "share", "antigravity-seo")
	}
}

// DriftDir is where drift snapshots live
func DriftDir() string {
	return filepath.Join(DataDir(), "drift")
}

// StatePath is the full path of the runtime-state manifest
func StatePath() string {
	return filepath.Join(DataDir(), StateFileName)
}

// DetectIntegrations reads optional credential env vars and detects PDF engines
func DetectIntegrations() Integrations {
	pdf := ""
	for _, bin := range []string{"weasyprint", "google-chrome", "chromium", "chromium-browser", "chrome", "msedge", "wkhtmltopdf"} {
		if path, err := exec.LookPath(bin); err == nil {
			pdf = filepath.Base(path)
			break
		}
	}

	hasGSC := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" ||
		os.Getenv("GSC_ACCESS_TOKEN") != "" ||
		os.Getenv("GOOGLE_ACCESS_TOKEN") != "" ||
		os.Getenv("GSC_CREDENTIALS_FILE") != "" ||
		os.Getenv("GSC_CREDENTIALS_JSON") != ""

	return Integrations{
		GoogleAPIKey: os.Getenv("GOOGLE_API_KEY") != "",
		IndexNowKey:  os.Getenv("INDEXNOW_KEY") != "",
		GSCAuth:      hasGSC,
		PDFRenderer:  pdf,
	}
}

// PluginRoot infers the plugin installation root from the running binary
// location (<root>/bin/seo-engine). Falls back to the working directory.
func PluginRoot() string {
	exe, err := os.Executable()
	if err == nil {
		root := filepath.Dir(filepath.Dir(exe))
		if _, err := os.Stat(filepath.Join(root, "plugin.json")); err == nil {
			return root
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}

// RunSetup creates the data directory, drift store, and runtime-state manifest.
func RunSetup(engineVersion, pluginRoot string) (*SetupReport, error) {
	report := &SetupReport{
		DataDir:      DataDir(),
		DriftDir:     DriftDir(),
		Adapters:     []string{},
		Integrations: DetectIntegrations(),
		Warnings:     []string{},
	}

	if err := os.MkdirAll(report.DriftDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed creating data dir %s: %w", report.DataDir, err)
	}
	report.StatePath = StatePath()

	state := RuntimeState{
		RuntimeSchema: RuntimeSchemaVersion,
		EngineVersion: engineVersion,
		DataDir:       report.DataDir,
		Adapters:      report.Adapters,
		Integrations:  report.Integrations,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(report.StatePath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("failed writing runtime state: %w", err)
	}

	return report, nil
}

// RunDoctor performs read-only readiness checks and never mutates state.
func RunDoctor(engineVersion string, offline bool) *DoctorReport {
	report := &DoctorReport{
		EngineVersion: engineVersion,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
		DataDir:       DataDir(),
		Checks:        []Check{},
		Integrations:  DetectIntegrations(),
	}
	ready := true

	// 1. Data directory
	if info, err := os.Stat(report.DataDir); err == nil && info.IsDir() {
		report.Checks = append(report.Checks, Check{"data_dir", StatusOK, report.DataDir})
	} else {
		report.Checks = append(report.Checks, Check{"data_dir", StatusFail, "missing — run `seo-engine setup`"})
		ready = false
	}

	// 2. Runtime state manifest
	raw, err := os.ReadFile(StatePath())
	if err != nil {
		report.Checks = append(report.Checks, Check{"runtime_state", StatusFail, "missing — run `seo-engine setup`"})
		ready = false
	} else {
		var state RuntimeState
		if err := json.Unmarshal(raw, &state); err != nil {
			report.Checks = append(report.Checks, Check{"runtime_state", StatusFail, "corrupt JSON"})
			ready = false
		} else {
			report.StatePresent = true
			switch {
			case state.RuntimeSchema != RuntimeSchemaVersion:
				report.Checks = append(report.Checks, Check{"runtime_state", StatusWarn,
					fmt.Sprintf("schema %d != expected %d — re-run setup", state.RuntimeSchema, RuntimeSchemaVersion)})
			case state.EngineVersion != engineVersion:
				report.Checks = append(report.Checks, Check{"runtime_state", StatusWarn,
					fmt.Sprintf("state written by v%s, running v%s — re-run setup", state.EngineVersion, engineVersion)})
			default:
				report.Checks = append(report.Checks, Check{"runtime_state", StatusOK, fmt.Sprintf("v%s", state.EngineVersion)})
			}
		}
	}

	// 3. Drift store
	if info, err := os.Stat(DriftDir()); err == nil && info.IsDir() {
		report.Checks = append(report.Checks, Check{"drift_store", StatusOK, DriftDir()})
	} else {
		report.Checks = append(report.Checks, Check{"drift_store", StatusWarn, "not initialized (drift snapshots unavailable until setup)"})
	}

	// 4. Execution Mode
	report.Checks = append(report.Checks, Check{"execution_mode", StatusOK, "shell (pure CLI native)"})

	// 5. Optional integrations
	if report.Integrations.GoogleAPIKey {
		report.Checks = append(report.Checks, Check{"google_api", StatusOK, "GOOGLE_API_KEY set — psi/crux commands enabled"})
	} else {
		report.Checks = append(report.Checks, Check{"google_api", StatusInfo, "GOOGLE_API_KEY not set — psi/crux commands degraded (optional)"})
	}
	if report.Integrations.IndexNowKey {
		report.Checks = append(report.Checks, Check{"indexnow", StatusOK, "INDEXNOW_KEY set — indexnow submission enabled"})
	} else {
		report.Checks = append(report.Checks, Check{"indexnow", StatusInfo, "INDEXNOW_KEY not set — indexnow submission degraded (optional)"})
	}
	if report.Integrations.GSCAuth {
		report.Checks = append(report.Checks, Check{"gsc_auth", StatusOK, "credentials set — gsc query/inspect commands enabled"})
	} else {
		report.Checks = append(report.Checks, Check{"gsc_auth", StatusInfo, "credentials not set — gsc query/inspect commands degraded (optional)"})
	}
	if report.Integrations.PDFRenderer != "" {
		report.Checks = append(report.Checks, Check{"pdf_engine", StatusOK, fmt.Sprintf("%s found in PATH — native PDF exports enabled", report.Integrations.PDFRenderer)})
	} else {
		report.Checks = append(report.Checks, Check{"pdf_engine", StatusInfo, "no renderer in PATH (weasyprint/chromium) — HTML reports supported, PDF degraded (optional)"})
	}

	// 6. Network reachability (skippable)
	if offline {
		report.Checks = append(report.Checks, Check{"network", StatusInfo, "skipped (--offline)"})
	} else {
		if err := probeNetwork(); err != nil {
			report.Checks = append(report.Checks, Check{"network", StatusWarn, fmt.Sprintf("unreachable: %v", err)})
		} else {
			report.Checks = append(report.Checks, Check{"network", StatusOK, "outbound HTTPS OK"})
		}
	}

	report.Ready = ready
	return report
}

func probeNetwork() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://www.google.com/generate_204", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
