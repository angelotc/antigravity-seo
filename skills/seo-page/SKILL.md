---
name: seo-page
description: Deep single-page SEO analysis combining transport, technical, schema, images, content E-E-A-T, and hreflang audits in one report. Use when the user asks to analyze one specific page or URL.
---

# Deep Page Audit

One command, six audit dimensions:

```bash
seo-engine page <url> [--keyword "target phrase"] --json
```

## What it reports
| Dimension | Checks |
|---|---|
| Headers | status, redirect chain, X-Robots-Tag, TTFB, HSTS |
| Technical | title/meta lengths, canonical, viewport, H1, link balance |
| Schema | JSON-LD validity, required properties per type, deprecated types |
| Images | alt coverage, width/height (CLS), formats, lazy-load, data URIs |
| Content | word count, heading hierarchy, byline/date E-E-A-T, answer blocks, keyword density |
| Hreflang | BCP47 codes, x-default, self-reference, duplicates |

## Interpretation guide
- **page_score** (avg of four sub-scores) below 70 → fix before content work.
- Answer blocks = 0 → restructure the intro as a direct 40–80 word answer.
- Schema errors are shipping blockers; warnings are ranking opportunities.

## MCP alternative
Other assistants can call the `seo_audit_page` tool via `serve-mcp` (headers+technical+schema) or compose `seo_audit_images` / `seo_audit_content` / `seo_audit_hreflang`.
