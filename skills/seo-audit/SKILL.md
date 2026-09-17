---
name: seo-audit
description: Full site-wide SEO audit orchestrator. Crawls the sitemap, samples key page types, runs parallel subagent audits, and produces a health scorecard. Use when the user asks to audit an entire site or domain.
---

# Site-Wide SEO Audit

Orchestrates a full-domain audit: transport health → sitemap integrity → sampled deep page audits → synthesized health report.

## Workflow

### Phase 1: Foundation (run directly)
```bash
seo-engine robots <domain> --json
seo-engine sitemap <domain>/sitemap.xml --limit 20 --json
seo-engine llms <domain> --json
```
Flag: global AI-crawler blocks, sitemap 404s/redirects, missing llms.txt.

### Phase 2: Sample selection
From the sitemap URL list, pick up to 10 URLs across distinct page types (home, hub/category, detail/article, landing). Prefer pages with the highest traffic or template diversity.

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
