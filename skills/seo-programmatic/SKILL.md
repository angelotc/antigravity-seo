---
name: seo-programmatic
description: Programmatic SEO page generation with doorway-page quality gates, template design, and internal linking patterns. Use when creating pages at scale from structured data.
---

# Programmatic SEO

Generating pages from databases is legitimate only when each page delivers unique value. Google's site-reputation-abuse and doorway policies are the failure modes.

## Quality gates (hard rules)
- **30 pages**: review checkpoint — if templates produce near-duplicate pages, stop and add differentiation.
- **50 pages**: hard stop until every page passes the uniqueness bar (below).
- **Uniqueness bar per page**: unique data (prices, availability, local specifics), unique intro (not string interpolation of the same sentence), and at least one element no sibling page has (chart, map, table, review summary).

## Page template design
1. Answer block first (40–80 words answering the page's head term)
2. Unique data visualization
3. Deep supporting content (200+ words, entity-rich)
4. Schema matching the page type (`schema` command validates)
5. Self-canonical, no auto noindex

## Validation loop
Generate → audit at scale:
```bash
seo-engine page <sample-url> --json   # repeat across template variants
seo-engine sitemap generate <section-url> --max-pages 100 --out /tmp/sitemap.xml
```
Sample 5–10 generated pages per template variant; a content score <60 on any variant means the template needs more injected uniqueness before scaling.

## Internal linking
Hub page links to every generated page; generated pages link back to the hub + 2–5 nearest siblings (by shared facet). Orphaned programmatic pages are dead weight — verify hub coverage with the generated sitemap.

## Publishing cadence
Roll out in batches (e.g. 50/week), monitoring indexation and rankings per batch; halt and reassess if indexation rate <50%.
