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
- Google Search Console: native Go service account RSA/PKCS8 JWT minting + bearer exchange, zero dependencies; supports `gsc query` (Search Analytics) and `gsc inspect` (URL Inspection).
- Executive PDF & HTML reports: standalone printable HTML report with CSS gauges and issue badges; renders directly to PDF via WeasyPrint or headless Chromium (`seo-engine report <url> --pdf`).
- Keyless Backlinks: queries public Common Crawl CDX index graph (`seo-engine backlinks <domain>`).
- Ops & Doctor: `doctor` detects GSC credentials and available PDF engines (`weasyprint` / `chromium`).
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
- `go build ./... && go vet ./... && go test ./...` green; `go mod tidy -diff` clean; single dependency (golang.org/x/net).
- Cross-compilation: GOOS=windows/amd64, darwin/arm64, linux/amd64 all build.
- Live smoke vs nipponhomes.com: robots (group attribution + disallow patterns), llms (95/100, llms.txt present), drift baseline/compare/history, deep `page` audit with keyword analysis (JSON verified parseable).
- Hook contract: placeholder file → exit 2 + findings; clean file → `{}` exit 0.
- MCP: initialize/tools/list handshake returns 8 tools.
- install.sh → symlink + setup + doctor(0) → uninstall.sh round-trip in temp INSTALL_DIR.
- `bash -n` clean on all shell scripts; ps1 files reviewed line-by-line (no pwsh on this host).

### Known deltas vs upstream (documented in README parity matrix)
- No Python/Playwright SPA rendering or screenshots (engine is static-HTML; use harness browser tools).
- No PDF report generation (Markdown artifacts).
- No GSC/GA4/Ads OAuth or paid MCP extensions (Ahrefs/DataForSEO/Moz/SE Ranking/Profound/Firecrawl).
- Drift store is JSON snapshots, not SQLite; Common Crawl backlink graph not implemented (methodology in seo-backlinks skill).
