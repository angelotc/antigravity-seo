# antigravity-seo parity build vs claude-seo v2.3.1

Audit verdict: local suite covered ~25% of the reference (no install/ops layer,
inert schema hook, 6/25 skills, 7 engine commands vs 32). This file tracks the
gap-closure work.

## Phases

- [x] Phase 1 — Ops layer: `setup`/`doctor` subcommands, install.sh, uninstall.sh, LICENSE, plugin.json 2.0.0
- [x] Phase 2 — Real schema-linter hook: `lint-schema-file` subcommand + wrapper script
- [x] Phase 3 — Bug fixes: robots parser rewrite (+tests), schema type expansion (+tests), MCP tools + check_limit clamp, go mod tidy
- [x] Phase 4 — New audits: images, content, hreflang, llms, page deep-audit (+tests)
- [x] Phase 5 — Sitemap generation + drift baseline/compare/history (+tests)
- [x] Phase 6 — Key-based integrations: psi, crux, indexnow (+tests, graceful no-key)
- [x] Phase 7 — 15 new skills + update existing 6 (21 total)
- [x] Phase 8 — README rewrite w/ parity matrix + verification suite
- [x] Phase 9 (follow-up) — OS-agnostic pass: per-OS data dirs, install.ps1/uninstall.ps1, PowerShell lint hook, de-hardcoded paths in skills/rules/adapters, flags-after-URL parsing bug fixed
- [x] Phase 10 (follow-up) — Website-agnostic pass: neutral Accept-Language (was ja-biased, now a ClientOptions field), CLI examples de-branded, seo-schema templates rewritten multi-vertical, drift/content/technical/images examples + rules wording generalized; live-verified on go.dev
- [x] Phase 11 — Google Search Console integration (Search Analytics query + URL Inspection, Service Account JWT & access token auth, unit tests)
- [x] Phase 12 — HTML & PDF audit report generator (executive scorecard, print-optimized CSS, WeasyPrint / headless Chrome runner, unit tests)
- [x] Phase 13 — Keyless Common Crawl backlink explorer (CDX index query, domain summaries, unit tests)
- [x] Phase 14 — Skills review & alignment (verify all 21 skills, wire new commands into seo-backlinks, seo-technical, seo-audit, update doctor & README)
- [x] Phase 15 — Verification, rebuild binary, commit & push (all tests green, live validation, attribution-free push)
- [x] Phase 16 — Pure Shell / CLI Conversion: remove serve-mcp & internal/mcp, clean adapters & runtime doctor, update skills & docs, rebuild & verify
- [ ] Phase 17 — Deslop & Authenticity Engine (Go heuristics in internal/audit/deslop.go, seo-engine deslop CLI, seo-deslop skill + references)
- [ ] Phase 18 — Persistent Project Memory (internal/context/store.go, seo-engine context CLI, seo-project-setup skill)
- [ ] Phase 19 — Specialized Workflows (seo-coach, seo-link-prospecting, seo-competitive-landscape skills)
- [ ] Phase 20 — GA4 & DataForSEO integration (internal/api/ga4.go with Google JWT, internal/api/dataforseo.go, CLI commands)
- [ ] Phase 21 — Verification suite and attribution-free release

## Review

### Delivered
- Pure Shell / CLI Conversion: Removed `serve-mcp` and `internal/mcp` package entirely; removed `adapters/` (Codex/Cursor MCP configs); simplified `seo-engine doctor` to verify native shell execution mode; updated skills and documentation to rely 100% on CLI shell execution.
- Engine: 18 → 21 CLI commands (`report`, `backlinks`, `gsc` added); skills: 21 verified and aligned; tests: 46 test funcs across 7 packages (all green).
- Google Search Console: native Go service account RSA/PKCS8 JWT minting + bearer exchange, no third-party auth library; supports `gsc query` (Search Analytics) and `gsc inspect` (URL Inspection).
- Executive PDF & HTML reports: standalone printable HTML report with CSS gauges and issue badges; native Go A4 PDF via fpdf (`seo-engine report <url> --pdf`); the WeasyPrint/Chromium runner was removed in Phase 22.
- Keyless Backlinks: queries public Common Crawl CDX index graph (`seo-engine backlinks <domain>`).
- Ops & Doctor: `doctor` detects GSC credentials; PDF rendering is native (no external engine).
- Ops parity with upstream #installation: install.sh/install.ps1, uninstall.sh/uninstall.ps1, `setup` (data dir + runtime-state.json), `doctor` (readiness + integration tiers), exit-code contract 0/10/1.
- Schema PostToolUse hook now blocks placeholders and deprecated types (bash + PowerShell variants); was a no-op stub.
- robots.txt parser rewritten: user-agent groups (leak fixed), Allow/Disallow longest-match precedence, `*`/`$` wildcards, Crawl-delay, per-bot effective-group attribution.
- OS-agnostic: engine cross-compiles (linux/darwin/windows verified), per-OS data dirs (XDG / ~/Library / %LOCALAPPDATA%), no absolute paths anywhere in skills/rules/adapters, agents resolve `seo-engine` per rules/AGENTS.md rule 0.

### Fixed en route
- `flag` stops at first positional — `--json` after the URL (the documented convention everywhere) was silently ignored since v1.0.0; replaced with flags-anywhere `parseFlags` in every command runner.
- CrUX client sent a bogus `method=` query param; removed.
- Crawl dedupe: root URL with/without trailing slash counted as two pages.
- Removed empty `adapters/claude/` dir that misreported as an adapter.

### Verification performed
- `go build ./... && go vet ./... && go test ./...` green; `go mod tidy -diff` clean; dependencies at the time: golang.org/x/net, go-pdf/fpdf.
- Cross-compilation: GOOS=windows/amd64, darwin/arm64, linux/amd64 all build.
- Live smoke vs nipponhomes.com: robots (group attribution + disallow patterns), llms (95/100, llms.txt present), drift baseline/compare/history, deep `page` audit with keyword analysis (JSON verified parseable).
- Hook contract: placeholder file → exit 2 + findings; clean file → `{}` exit 0.
- MCP: initialize/tools/list handshake returns 8 tools.
- install.sh → symlink + setup + doctor(0) → uninstall.sh round-trip in temp INSTALL_DIR.
- `bash -n` clean on all shell scripts; ps1 files reviewed line-by-line (no pwsh on this host).

### Known deltas vs upstream (documented in README parity matrix)
- No Python/Playwright SPA rendering or screenshots (engine is static-HTML; use harness browser tools).
- No GSC/GA4/Ads OAuth or paid MCP extensions (Ahrefs/DataForSEO/Moz/SE Ranking/Profound/Firecrawl).
- Drift store is JSON snapshots, not SQLite; Common Crawl inbound-link graph not implemented (`backlinks` lists the domain's own CDX captures) (methodology in seo-backlinks skill).

## Phase 22 — Staff-review remediation + international text layer

Source: external staff review (verified item-by-item) + independent review additions.

### P0 — Honest packaging
- [x] Untrack `bin/seo-engine` (aarch64 ELF), add `.gitignore`
- [x] Single-source version (ldflags `-X`) + CI consistency check; Go 1.26 everywhere
- [x] README/marketplace truth pass (deps, "10ms", --json caveat, backlinks → CC capture index, hooks)
- [x] Delete dead WeasyPrint/Chromium pipeline; fix `--pdf` help text

### P1 — Correct output
- [x] Charset decoding (Shift_JIS/EUC-JP → UTF-8) in fetch path
- [x] Thread FinalURL through report + crawler; merge Headers.Issues into report
- [x] Robots: seed URL check, 5xx → disallow-all, Crawl-delay enforced; X-Robots-Tag noindex excluded from sitemaps
- [x] `internal/textutil`: script detection, script-aware length/reading time/thin content, display-width title/desc limits, NFKC + whole-word/CJK keyword matching
- [x] `internal/urlnorm`: canonical/hreflang compare + crawler dedupe (IDN, percent-encoding, default port, tracking params)
- [x] Drift key + TTFB noise; CrUX labeled/sorted history; Lighthouse rounding; truncation flag; per-hop timings; 401/403 ≠ broken
- [x] Schema/link fixes: placeholder penalty per block, offers arrays, protocol-relative links, anchors, multi-node title, deterministic hreflang order
- [x] CJK-safe PDF (embedded JP-capable font, rune-safe truncation)

### P2 — Safe
- [x] Installer checksum verification + pinned version; create ~/.local/bin
- [x] SSRF: dial validated IP, validate target when proxied
- [x] Google API key → header; IndexNow key redacted from --json; SHA-pin CI actions
- [x] Hook fires in Claude Code/OpenCode (Write/Edit matchers); wire or de-scope PowerShell hook

### Deferred (P3)
- GSC token cache, `--fail-on`, `--limit` clamp, HTML-first report

### Review

**Delivered** — every P0/P1/P2 item above. Highlights:
- New `internal/urlnorm` (IDN, percent-encoding, default ports, tracking params) and `internal/textutil` (script detection, words-vs-characters length, per-script thresholds, display-width title/description limits, NFKC whole-word/CJK keyword matching, rune-safe truncation).
- Fetch path decodes Shift_JIS/EUC-JP to UTF-8, flags truncation, records per-hop timings; SSRF guard dials the validated IP and validates the target when proxied.
- Crawler: FinalURL-based pages, robots seed check + 5xx disallow-all + Crawl-delay, X-Robots-Tag/`none` noindex exclusion, normalized dedupe; sitemap health splits 401/403/429 from broken.
- Report: audits the post-redirect URL, merges header issues (incl. X-Robots-Tag noindex and 4xx/5xx status) into scoring; native PDF embeds M PLUS 1p (OFL) so Japanese renders; dead WeasyPrint/Chromium path removed.
- Drift key no longer lowercases paths (legacy-key fallback); TTFB noise no longer flips `Changed`. CrUX history labeled + sorted; Lighthouse rounded; Google key moved to `X-goog-api-key`; IndexNow key redacted.
- Packaging: binary untracked + `.gitignore`; version via ldflags + CI consistency job; Go 1.26 everywhere; actions SHA-pinned; installers pin the version and verify SHA256SUMS; hook fires for Antigravity/Claude Code/OpenCode names and never blocks on a broken engine; README truth pass.

**Fixed during integration** — hook scripts had started discarding engine stderr (block reasons vanish); restored. 401/403/other 4xx lost their status finding when header issues replaced the old status check; added a CRITICAL catch-all + test. PDF benchmark column still quoted Latin character targets; now shows the width rule in use.

**Verification** — `go vet ./...` clean; `go test -race ./...` green (9 packages); cross-compile linux/amd64, darwin/arm64, windows/amd64; CI version-consistency step run locally; hook: placeholder → exit 2 with reason on stderr, clean → `{}`, broken engine → `{}` exit 0. Live: Shift_JIS page (abehiroshi.la.coocan.jp) title decoded correctly, script=cjk; nipponhomes.com/ja content measured as 1783 characters (was ~1 "word"), PDF renders Japanese (pdftotext round-trip + visual check).

**Not verified** — PowerShell scripts (no pwsh on host); installer against a real release (next tag must publish SHA256SUMS.txt, which release.yml already does).

**Costs** — binary grew ~13MB → ~18MB (embedded JP font ~3.4MB + x/text tables).

---

# Review follow-ups (2026-09-29)

Source: UX/functionality review (CLI hands-on + engine + skills passes). Scope agreed: suggested-order items 1-3, plus drop Claude Code/OpenCode support (Antigravity-only).

## Plan

### A — Agent-facing correctness (cmd/seo-engine, internal/api)
- [x] PSI: request all 4 Lighthouse categories (`q.Add`, not `q.Set`)
- [x] CrUX: decode real API shape (`percentiles` object, p75 number|string; history `percentilesTimeseries.p75s` with nulls); origin detection accepts `/`; 404 → friendly "no CrUX data"; fixtures match documented responses
- [x] `--json` errors: every `fatal` emits `{"error":{command,message,hint}}` on stdout when `--json` is present
- [x] `report`: progress banner to stderr (never pollutes `--json` stdout); help text matches real default filename
- [x] `--fail-on critical|warning` on issue-producing commands → exit 3; `drift compare --fail-on change` → exit 3; `lint-schema-file <missing path>` → exit 1 (hook stdin mode unchanged)
- [x] Exit-code contract documented in usage: 0 ok, 1 error, 2 flag/usage (Go flag), 3 gate tripped

### B — Sitemap library (internal/crawler/sitemap.go)
- [x] Follow `<sitemapindex>` children (bounded concurrency, `--max-sitemaps` cap, nested-index + cycle guard, per-child counts/errors, `complete` flag)
- [x] Discover sitemaps for a bare domain/HTML URL: robots.txt `Sitemap:` → `/sitemap.xml` → `/sitemap_index.xml`
- [x] Non-200 sitemap → error; gzip by magic bytes
- [x] `sample_urls` = `--limit` URLs spread round-robin across child sitemaps; health check uses that same sample; clamp `--limit` to 100

### C — Site crawl audit (new `seo-engine crawl`)
- [x] Refactor crawl core (`Crawl` with options + per-visit callback, depth, outlinks, all statuses); `CrawlSite` becomes a wrapper (sitemap generate unchanged)
- [x] Robots matching includes the query string
- [x] `internal/audit/site.go`: cross-page checks — broken internal links, internal redirects/chains, duplicate titles/descriptions/content, canonical → non-200/noindex/redirect, hreflang reciprocity + dead targets, sitemap cross-check (noindex/non-200/redirect/non-canonical in sitemap, indexable pages missing from sitemap, orphans only when crawl complete), deep pages
- [x] CLI: `crawl <url> [--max-pages 100] [--concurrency 4] [--sitemap URL|--no-sitemap] [--fail-on] [--json]`; wire B into `sitemap` command; skills (seo-audit, seo, seo-technical) use `crawl`

### D — Antigravity-only
- [x] Drop Claude Code/OpenCode/Codex from README, hooks.json matcher, seo-schema skill

### Verify
- [x] `go vet ./... && go test ./...` green; cross-compile linux/darwin/windows
- [x] Live: psi/crux shape (fixtures from docs; no valid key on VM), `sitemap https://nipponhomes.com` (index followed, sample spread), `crawl https://nipponhomes.com --max-pages 50`, `report --json | jq`, `--fail-on` exit codes, JSON error envelope

## Review

**Delivered**
- A: PSI now requests all 4 Lighthouse categories. The CrUX decoder matches the documented API: `percentiles` is an object, p75 is a number or string, history is `percentilesTimeseries.p75s` with nulls. A CrUX 404 returns `no_data` with exit 0. `--json` errors go to stdout as `{"error":{command,message,hint}}`. The `report` banner goes to stderr. `--fail-on critical|warning` exits 3 on 9 commands, and `drift compare --fail-on change` does the same. `lint-schema-file <missing>` exits 1. Usage has an EXIT CODES section. Google API errors are summarized to one line.
- B: the sitemap library follows index trees with a child cap, a depth limit of 3 and cycle detection. It auto-discovers sitemaps from robots.txt, then /sitemap.xml, then /sitemap_index.xml. It detects gzip by magic bytes, and rejects non-200 and HTML responses clearly. The sample is spread round-robin across children and evenly within each child, and it is the same set that gets health-checked. `total_urls` counts every URL, even past the 200k storage cap. `--max-sitemaps` added. The sitemap client allows 52 MB bodies.
- C: new `seo-engine crawl`:
  - An options-based `Crawl` core. `CrawlSite` is now a wrapper around it, so `sitemap generate` is unchanged.
  - `audit.AuditSite`/`AnalyzeSite` cover broken internal links, links to redirects and redirect chains, missing or duplicate titles and descriptions, duplicate content, canonical targets, hreflang return links, sitemap coverage and orphans, click depth, and robots-blocked links.
  - Orphans are gated on crawl completeness, and missing-from-sitemap on sitemap completeness.
  - Skills (seo-audit, seo, seo-technical) and the README are updated.
- D: Antigravity-only. The Claude Code, OpenCode and Codex docs are removed, and the hook matcher is limited to Antigravity's write tools.

**Fixed during integration**
- Data race in `SafeClient.Fetch`: the transport can finish a background dial after `Do` returns, and its httptrace callbacks then wrote `timings` while `Fetch` read it. The trace state is now guarded by a mutex. This also cleared an intermittent `-race` failure in `TestInspectSitemapSampling`.
- The crawl was nondeterministic: two runs on the same site fetched different page sets. Workers raced for the remaining budget, and links and redirect targets were recorded in the order fetches finished. Now the budget is assigned before any fetch starts, and the next batch is built only after the whole batch finishes, in URL order. Verified with identical page sets across two live nipponhomes runs.
- Canonical checks skip noindex source pages. nipponhomes' noindexed `/explore?…` filter variants had been reported as 5 CRITICAL issues.
- HTTP 503 is treated as blocked/unverified in both `crawl` and the sitemap health check, via the shared `crawler.UnverifiedStatus`. go.dev's `/change` link, which redirects to a googlesource host that intermittently returns 503, had flipped the crawl's exit code between runs.
- Sitemap output printed one error line per child skipped by the cap (56 on nipponhomes). These are now summarized in a single line.

**Verification**
- gofmt clean. `go vet ./...` clean.
- `go test -p 2 ./...` and `-race ./...` green.
- Cross-compiles for linux/amd64, darwin/arm64 and windows/amd64.
- Live checks:
  - JSON error envelope: exit 1, empty stderr.
  - `--fail-on`: 3 on a 404, 0 on a clean page, 1 on an invalid value.
  - Hook mode on a missing file: `{}` and exit 0.
  - `report --json` and `crawl --json` produce valid JSON.
  - `sitemap https://nipponhomes.com`: discovered from robots.txt, 950,019 URLs, sample spread across child sitemaps.
  - `crawl nipponhomes.com --max-pages 40`: deterministic, 0 critical.
  - `crawl go.dev --max-pages 80`: 35 links pointing at redirects, which are real (spot-checked).

**Not verified**
- A successful PSI or CrUX call: the GOOGLE_API_KEY on this VM is invalid. The fixtures follow the documented response shapes.
- `multi_replace_file_content` in the hook matcher is assumed to be an Antigravity tool name.
- hooks.json's top-level key and relative command path are unchanged and unverified against Antigravity's hook loader.

**Follow-ups**
- A crawl with the sitemap check on a very large site (950k URLs) peaks at about 420 MB RSS and takes about 29 s, versus about 10 s with `--no-sitemap`. Consider a lower default for `--max-sitemaps` in `crawl`, or storing only URL keys.
