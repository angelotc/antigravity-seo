// Package drift provides baseline/compare/history page-change monitoring
// using JSON snapshots in the engine data directory (zero external deps).
package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"antigravity-seo/internal/audit"
	"antigravity-seo/internal/crawler"
	"antigravity-seo/internal/runtime"
	"antigravity-seo/internal/urlnorm"
)

// Snapshot captures the SEO-relevant state of one page at one point in time
type Snapshot struct {
	URL             string    `json:"url"`
	Timestamp       time.Time `json:"timestamp"`
	ContentSHA256   string    `json:"content_sha256"`
	StatusCode      int       `json:"status_code"`
	Title           string    `json:"title"`
	MetaDescription string    `json:"meta_description"`
	Canonical       string    `json:"canonical"`
	H1              []string  `json:"h1"`
	SchemaTypes     []string  `json:"schema_types"`
	WordCount       int       `json:"word_count"`
	ImageCount      int       `json:"image_count"`
	InternalLinks   int       `json:"internal_links"`
	TTFBMS          int64     `json:"ttfb_ms"`
}

// Diff is one field-level change between two snapshots
type Diff struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// CompareReport is the full drift comparison of a page against its baseline
type CompareReport struct {
	URL          string    `json:"url"`
	BaselineTime time.Time `json:"baseline_time"`
	CurrentTime  time.Time `json:"current_time"`
	Changed      bool      `json:"changed"`
	Changes      []Diff    `json:"changes"`
}

// urlKey returns the dedupe key used to name snapshot files for a URL. It
// uses the shared urlnorm.Key normalization (case-sensitive path, IDN host,
// tracking-param stripping) so e.g. /About and /about are tracked as
// distinct pages instead of colliding on a single history.
func urlKey(rawURL string) string {
	norm := urlnorm.Key(rawURL, nil)
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:16]
}

// legacyURLKey reproduces the pre-urlnorm key scheme (whole URL lowercased,
// trailing slash trimmed). History/Latest fall back to it so snapshots
// captured before the urlnorm.Key switch are not silently orphaned.
func legacyURLKey(rawURL string) string {
	norm := strings.ToLower(strings.TrimRight(rawURL, "/"))
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])[:16]
}

func snapshotPath(s Snapshot) string {
	return filepath.Join(runtime.DriftDir(), fmt.Sprintf("%s-%d.json", urlKey(s.URL), s.Timestamp.Unix()))
}

// Capture builds a Snapshot from a live fetch result
func Capture(rawURL string, res *crawler.FetchResult) Snapshot {
	sum := sha256.Sum256(res.Body)
	snap := Snapshot{
		URL:           rawURL,
		Timestamp:     time.Now().UTC(),
		ContentSHA256: hex.EncodeToString(sum[:]),
		StatusCode:    res.StatusCode,
		TTFBMS:        res.Timings.TTFBMS,
		H1:            []string{},
		SchemaTypes:   []string{},
	}
	if htmlAudit, err := audit.InspectHTML(res.FinalURL, res.Body); err == nil {
		snap.Title = htmlAudit.Title
		snap.MetaDescription = htmlAudit.MetaDescription
		snap.Canonical = htmlAudit.Canonical
		snap.H1 = htmlAudit.H1Text
		snap.ImageCount = htmlAudit.TotalImages
		snap.InternalLinks = htmlAudit.InternalLinks
	}
	if contentAudit, err := audit.InspectContent(res.FinalURL, res.Body, ""); err == nil {
		snap.WordCount = contentAudit.WordCount
	}
	if schemaAudit := audit.InspectSchema(res.FinalURL, res.Body); schemaAudit != nil {
		snap.SchemaTypes = schemaAudit.TypesFound
	}
	return snap
}

// Save persists a snapshot and returns its file path
func Save(snap Snapshot) (string, error) {
	if err := os.MkdirAll(runtime.DriftDir(), 0o755); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	p := snapshotPath(snap)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// Baseline captures and persists a new snapshot for a URL
func Baseline(rawURL string, res *crawler.FetchResult) (*Snapshot, string, error) {
	snap := Capture(rawURL, res)
	p, err := Save(snap)
	if err != nil {
		return nil, "", err
	}
	return &snap, p, nil
}

// History lists all snapshots for a URL, oldest first. It looks up
// snapshots under the current urlnorm.Key scheme first, falling back to the
// legacy (naive lowercase) key when that has no history, so pages baselined
// before the urlnorm.Key switch keep their existing snapshots.
func History(rawURL string) ([]Snapshot, error) {
	entries, err := os.ReadDir(runtime.DriftDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []Snapshot{}, nil
		}
		return nil, err
	}

	snaps := collectSnapshots(entries, urlKey(rawURL)+"-")
	if len(snaps) == 0 {
		snaps = collectSnapshots(entries, legacyURLKey(rawURL)+"-")
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Timestamp.Before(snaps[j].Timestamp) })
	return snaps, nil
}

func collectSnapshots(entries []os.DirEntry, prefix string) []Snapshot {
	var snaps []Snapshot
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(runtime.DriftDir(), e.Name()))
		if err != nil {
			continue
		}
		var s Snapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		snaps = append(snaps, s)
	}
	return snaps
}

// Latest returns the most recent baseline snapshot for a URL
func Latest(rawURL string) (*Snapshot, error) {
	snaps, err := History(rawURL)
	if err != nil {
		return nil, err
	}
	if len(snaps) == 0 {
		return nil, fmt.Errorf("no baseline snapshot for %s — run `drift baseline` first", rawURL)
	}
	return &snaps[len(snaps)-1], nil
}

// Compare diffs a fresh capture against the latest stored baseline
func Compare(rawURL string, res *crawler.FetchResult) (*CompareReport, error) {
	base, err := Latest(rawURL)
	if err != nil {
		return nil, err
	}
	current := Capture(rawURL, res)

	report := &CompareReport{
		URL:          rawURL,
		BaselineTime: base.Timestamp,
		CurrentTime:  current.Timestamp,
		Changes:      []Diff{},
	}

	significant := false
	diff := func(field, before, after string) {
		if before != after {
			report.Changes = append(report.Changes, Diff{Field: field, Before: before, After: after})
			significant = true
		}
	}
	diffInt := func(field string, before, after int) {
		diff(field, fmt.Sprintf("%d", before), fmt.Sprintf("%d", after))
	}

	diff("status_code", fmt.Sprintf("%d", base.StatusCode), fmt.Sprintf("%d", current.StatusCode))
	diff("content_sha256", base.ContentSHA256[:12], current.ContentSHA256[:12])
	diff("title", base.Title, current.Title)
	diff("meta_description", base.MetaDescription, current.MetaDescription)
	diff("canonical", base.Canonical, current.Canonical)
	diff("h1", strings.Join(base.H1, " | "), strings.Join(current.H1, " | "))
	diff("schema_types", strings.Join(base.SchemaTypes, ", "), strings.Join(current.SchemaTypes, ", "))
	diffInt("word_count", base.WordCount, current.WordCount)
	diffInt("image_count", base.ImageCount, current.ImageCount)
	diffInt("internal_links", base.InternalLinks, current.InternalLinks)

	// TTFB is noisy (network jitter, CDN cache state, cold vs warm
	// connections) and would otherwise flip Changed on almost every run.
	// Report any shift informationally, but only let it count toward
	// Changed when it is both large in absolute terms (>500ms) and large
	// relative to the baseline (>50%, or the baseline was ~0ms).
	if base.TTFBMS != current.TTFBMS {
		report.Changes = append(report.Changes, Diff{
			Field:  "ttfb_ms",
			Before: fmt.Sprintf("%dms", base.TTFBMS),
			After:  fmt.Sprintf("%dms", current.TTFBMS),
		})
		delta := current.TTFBMS - base.TTFBMS
		if delta < 0 {
			delta = -delta
		}
		if delta > 500 && (base.TTFBMS == 0 || float64(delta) > float64(base.TTFBMS)*0.5) {
			significant = true
		}
	}

	report.Changed = significant
	return report, nil
}
