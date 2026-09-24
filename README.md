# Antigravity SEO & GEO Suite

An SEO and Generative Engine Optimization (GEO) suite for **Google Antigravity**, powered by Go.

> [!NOTE]
> **Lineage**: Inspired by [`claude-seo`](https://github.com/AgriciDaniel/claude-seo) by Daniel Agrici. Antigravity SEO replaces Python/Playwright runtimes with a single Go binary (`seo-engine`) and native agent tools (`search_web`, `read_url_content`).

---

## claude-seo + Claude Opus 5 vs. antigravity-seo + Gemini 3.8 Flash

| Dimension | claude-seo + Claude Opus 5 | antigravity-seo + Gemini 3.8 Flash | Advantage |
|---|---|---|---|
| **Input / Output Token Cost** | $5.00 / $25.00 per 1M | **$0.75 / $3.75 per 1M** | **6.7x cheaper** |
| **Single Page Audit** (~50k in / 10k out) | ~$0.50 | **~$0.075** | **~85% savings** |
| **Full Site Audit** (20 pages, ~400k in / 80k out) | **~$4.00 – $8.00+** | **~$0.60** | **~85% – 90% savings** |
| **Multi-Agent Swarm Crawls** | Not supported (single model tier) | **~$0.12 – $0.20** (via `flash_lite`) | **~25x – 40x cheaper** |
| **Live SERP & Crawl Data** | Optional paid SaaS MCPs (DataForSEO, Firecrawl, Ahrefs) | ✅ **Native `search_web` & Common Crawl** | **Zero paid subscriptions** |
| **Engine Runtime** | Python venv, pip dependencies, Playwright | ✅ **Single Go binary (`seo-engine`)** | **No Python venv or browser daemon; small pure-Go dependency footprint** |
| **Google Search Console** | External Node/Python scripts | ✅ **Native RSA JWT exchange** | **Built-in auth & queries** |

*Pricing based on September 2026 rates. Multi-agent crawls fan out to `flash_lite` subagents for penny-scale execution.*

### Key Architectural Differences

1. **Token Economics**: Claude Opus 5 thinking traces bill at $25/1M, making multi-page audits costly ($4–$8+). Gemini 3.8 Flash ($0.75 / $3.75) cuts costs by 85–90% (~$0.60), while `flash_lite` subagent swarms handle raw crawling for <$0.15.
2. **Small Pure-Go Engine**: `seo-engine` is a standalone binary (deps: `golang.org/x/net`, `golang.org/x/text`, `github.com/go-pdf/fpdf`) that eliminates Python venvs, pip modules, and headless browser daemons.
3. **Native Grounding**: Live Google SERP analysis and markdown extraction use Antigravity's built-in `search_web` and `read_url_content` without paid third-party MCPs.
4. **Built-in GSC & Backlinks**: Direct RSA/PKCS8 JWT authentication for Google Search Console and keyless Common Crawl CDX index lookups.
5. **Native A4 PDF Reports**: Built-in PDF report generation (`--pdf`) via `fpdf` with an embedded font covering Latin + Japanese — no external renderer (no WeasyPrint, no headless Chromium) needed.

---

## Architecture

```mermaid
flowchart LR
    User([User]) --> Skills["21 Antigravity Skills\nAudit • Schema • Content • GEO • Backlinks"]
    Skills --> Engine["Go Engine (seo-engine)\nFast audits, crawler, GSC, & PDF generation"]
    Skills --> Native["Native Grounding\nsearch_web & read_url_content"]
    Engine --> Output["Deliverables\nCLI • JSON • Enterprise A4 PDF"]
```

---

## Installation

### One-Line Install (Recommended)

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/angelotc/antigravity-seo/main/install.sh | bash
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/angelotc/antigravity-seo/main/install.ps1 | iex
```

Installs the plugin to `~/.gemini/config/plugins/antigravity-seo` and symlinks `seo-engine` into `~/.local/bin` (created if missing; the installer warns if that directory isn't on your `PATH`).

By default the installer fetches a pinned release version (matching this checkout's `plugin.json`) rather than `releases/latest`, and verifies the downloaded binary's SHA256 against that release's `SHA256SUMS.txt` before installing it — the install aborts on a checksum mismatch. Override the version with `SEO_ENGINE_VERSION` (`$env:SEO_ENGINE_VERSION` on Windows):

```bash
SEO_ENGINE_VERSION=2.1.0 bash install.sh
```

### Install from Source

```bash
git clone https://github.com/angelotc/antigravity-seo.git
cd antigravity-seo
bash install.sh          # On Windows: powershell -ExecutionPolicy Bypass -File install.ps1
```

Verify anytime with:
```bash
seo-engine doctor
```

---

## Commands

| Command | Purpose |
|---|---|
| `headers <url>` | HTTP status, redirect chains, X-Robots-Tag, TTFB |
| `audit <url>` | On-page technical + schema audit |
| `page <url>` | Deep audit (technical + schema + images + content + hreflang) |
| `report <url>` | Executive HTML or native A4 PDF report (`--pdf`) |
| `schema <url>` | JSON-LD validation vs Google Rich Results (14+ types) |
| `images <url>` | Alt coverage, dimensions, CLS, formats, lazy loading |
| `content <url>` | Word count, headings, E-E-A-T signals, answer blocks |
| `hreflang <url>` | BCP47 codes, x-default, self-reference, duplicates |
| `llms <url>` | `/llms.txt` and `/llms-full.txt` AI discovery audit |
| `sitemap <url>` | Sitemap validation (or `sitemap generate <url>`) |
| `robots <url>` | Robots.txt directives and AI crawler access policies |
| `drift <url>` | Change monitoring: `baseline`, `compare`, `history` |
| `backlinks <domain>` | Free Common Crawl capture index — the domain's own archived URLs, not inbound links |
| `gsc query\|inspect` | Google Search Console Search Analytics and URL Inspection |
| `psi <url>` / `crux <url>` | PageSpeed Insights and Chrome UX Report field data |
| `indexnow <url...>` | Instant submission to Bing, Yandex, Seznam, and Naver |

All commands support `--json`. Core audit features are 100% keyless.

---

## Skills (21)

- **Orchestration**: `seo`, `seo-audit`, `seo-page`
- **Technical & Content**: `seo-technical`, `seo-content`, `seo-schema`, `seo-geo`, `seo-images`, `seo-hreflang`, `seo-drift`, `seo-backlinks`
- **Search & Local**: `seo-local`, `seo-maps`, `seo-ecommerce`, `seo-cluster`, `seo-sxo`
- **Strategy**: `seo-plan`, `seo-programmatic`, `seo-competitor-pages`, `seo-content-brief`, `seo-flow`

The plugin also includes a **PostToolUse schema hook** — see [Hooks](#hooks) below.

---

## Hooks

`hooks.json` registers a PostToolUse schema quality gate (`hooks/schema_linter.sh`) that validates JSON-LD on file writes: it **blocks** (exit 2) placeholder values and deprecated schema types, and **warns** on invalid JSON or a missing `@context`/`@type`. The hook's `matcher` fires on Antigravity's own write tools (`write_to_file`, `replace_file_content`) as well as Claude Code's (`Write`, `Edit`, `MultiEdit`) and OpenCode's (`write`, `edit`), so it runs unmodified in any of those three harnesses. It never blocks an edit because of an engine problem — a missing, wrong-arch, or crashing `seo-engine` binary always falls through to `{}` / exit 0.

**Windows**: `hooks.json` has no field for an OS-specific command, so it always shells out to the bash script above. A `hooks/schema_linter.ps1` counterpart ships with the same contract but is **not wired in automatically** — if your shell can't run Bash (no WSL/Git Bash on `PATH`), edit `hooks.json` and repoint `command` at it:

```json
"command": "powershell -NoProfile -ExecutionPolicy Bypass -File ./hooks/schema_linter.ps1"
```

---

## International Text Support

The engine measures and audits pages on their own terms instead of assuming space-delimited Latin text everywhere:

- **Script-aware content metrics** (`internal/textutil`): classifies page text by script (Latin, CJK, Hangul, or other non-space-delimited scripts like Thai/Lao/Khmer) and switches word-count vs. character-count measurement accordingly, so a Japanese or Chinese page isn't scored as "thin content" by a word-splitting heuristic that doesn't apply to it.
- **Display-width title/description limits**: SERP snippet-length checks account for full-width CJK/Hangul glyphs rendering roughly twice as wide as Latin characters, instead of a flat character-count cutoff.
- **Unicode URL normalization** (`internal/urlnorm`): canonical/hreflang comparison and crawl dedupe resolve IDN hosts to punycode, normalize percent-encoding, and strip default ports — so `例え.jp` and `xn--r8jz45g.jp` compare equal.
- **Charset decoding**: non-UTF-8 encodings (Shift_JIS, EUC-JP) are decoded before parsing so byte-level mojibake doesn't corrupt text audits on pages that declare a legacy Japanese charset.

---

## Universal Shell / CLI Integration

The engine and skills run 100% natively via standard shell execution (`seo-engine <command> [options] <url> --json`) with zero MCP middleware. Simply ensure `seo-engine` is in your `$PATH` (or run `install.sh`).

- **Antigravity**: Native execution via `run_command`
- **OpenCode**: Register skills path in `opencode.json`:
  ```json
  "skills": {
    "paths": ["~/.gemini/config/plugins/antigravity-seo/skills"]
  }
  ```
- **Claude Code**: Symlink or copy skills into `~/.claude/skills/`
- **Codex / Cursor / Aider**: Invoke `seo-engine` directly via the terminal / bash tool

---

## Upstream Parity vs [claude-seo](https://github.com/AgriciDaniel/claude-seo)

| Upstream Capability | Antigravity SEO Implementation |
|---|---|
| Runtime & Doctor | ✅ Pure Go runtime; zero Python venv |
| Audits (Technical, Schema, Content, Images, Hreflang) | ✅ Fast engine commands + 21 specialized skills |
| Sitemap Generator | ✅ Built-in crawler: `sitemap generate` |
| Schema Quality Gate | ✅ PostToolUse hook, wired for bash (Antigravity/Claude Code/OpenCode); PowerShell script ships but needs manual wiring on Windows |
| Drift Tracking | ✅ Dependency-free JSON snapshot store (no SQLite) |
| Search Console (GSC) | ✅ Native RSA JWT service account exchange |
| Backlink Analysis | ✅ Free Common Crawl CDX index query |
| PDF Reporting | ✅ Pure Go A4 PDF engine (`fpdf`), no external renderer required |
| SERP Grounding | ✅ Native Antigravity `search_web` (no paid MCP accounts needed) |
| SPA JavaScript Rendering | 🔁 Delegated to Antigravity's native browser tools |

---

## License

MIT — see [LICENSE](LICENSE).
