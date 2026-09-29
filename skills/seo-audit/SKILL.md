---
name: seo-audit
description: Full site-wide SEO audit orchestrator. Crawls the sitemap, samples key page types, runs parallel subagent audits, and produces a health scorecard. Use when the user asks to audit an entire site or domain.
---

# Site-Wide SEO Audit

Orchestrates a full-domain audit: transport health → site-wide crawl → sitemap sample → deep page audits → synthesized health report.

## Workflow

### Phase 1: Foundation (run directly)
```bash
seo-engine robots <domain> --json
seo-engine llms <domain> --json
```
Flag: global AI-crawler blocks, missing llms.txt.

### Phase 2: Discovery
1. Site-wide issues in one pass (broken links, duplicate titles/content, canonical and hreflang problems, sitemap coverage, orphan pages):
   ```bash
   seo-engine crawl <site-root> --max-pages 100 --json
   ```
   Read `issues` (sorted by severity, each with a `fix` and up to 20 `affected` URLs) and `notes` (checks that were skipped, e.g. orphans when the crawl hit `--max-pages`). For CI-style gating add `--fail-on critical` (or `warning`); the exit code is 3 when the threshold is met.
2. URLs for deep page audits. `sample_urls` is spread evenly across child sitemaps (not the first URLs listed), and `--limit` is capped at 100:
   ```bash
   seo-engine sitemap <site-root> --limit 20 --json
   ```
   Pass the site root: it follows sitemap indexes and auto-discovers the sitemap. Flag sitemap 404s/redirects from `broken_urls` and `redirecting_urls`, and treat `complete: false` as a partial view (raise `--max-sitemaps`).

### Phase 2b: Sample selection
From `sample_urls`, pick up to 10 URLs across distinct page types (home, hub/category, detail/article, landing). Prefer pages with the highest traffic or template diversity.

### Phase 3: Parallel page audits (fan out with `invoke_subagent`)
For each sampled URL, one subagent runs:
```bash
seo-engine page <url> --json
```
Use flash/flash_lite tier subagents for crawl-heavy work; reserve pro tier for synthesis.

### Phase 4: Synthesis (UI Artifact)
Produce a scorecard artifact with:
1. **Site Health Score** = average of page scores, weighted by template (home ×3, hubs ×2, details ×1).
2. **Frequency analysis**: issues appearing on multiple pages are template bugs (fix once); single-page issues are content bugs.
3. **Prioritized fixes**: `[CRITICAL]` indexation/crawl blockers first, `[WARNING]` performance/schema, `[TIP]` GEO/E-E-A-T.
4. **Mermaid diagram** of the crawl hierarchy with per-node status color.
5. **Client Deliverable**: Render printable executive audit reports with `seo-engine report <url> --pdf --out audit.pdf`.
