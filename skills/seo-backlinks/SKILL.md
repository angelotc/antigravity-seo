---
name: seo-backlinks
description: Backlink and off-page SEO analysis — link quality assessment, free discovery methods, toxic link review, and linkable-asset strategy. Use for backlink, linking domain, or off-page authority questions.
---

# Backlinks & Off-Page Authority

## 1. Quality framework (evaluate any link)
Score each linking domain on:
- **Relevance**: topical overlap with the target site (a cooking blog linking to real estate = weak)
- **Authority**: organic traffic of the linking page (use `search_web` to check indexing), editorial standards
- **Placement**: in-content editorial links > author bios > footers > sitewides
- **Follow status**: `nofollow`/`sponsored`/`ugc` pass little equity but can drive traffic and diversity

## 2. Free discovery methods (keyless & open)
- **Engine Common Crawl query**: `seo-engine backlinks <domain> --limit 50 --json` (or via MCP `seo_query_backlinks`) queries the open Common Crawl CDX index for domain presence, crawled captures, and MIME distribution without paid subscriptions.
- `search_web`: `"{{domain}}" -site:{{domain}}` surfaces mentions; unlinked mentions are link-building leads.
- Competitor overlaps: for each top competitor, `search_web` for their brand mentions and guest-post footprints (`"{{competitor}}" "guest post"`).
- Internal: the engine's sitemap audit reveals internal-link equity distribution to strengthen externally-linked money pages.

## 3. Toxic link review
Patterns justifying disavow consideration (only after manual-action suspicion): sitewide spam anchors, foreign-language irrelevant sites, link networks with exact-match commercial anchors at scale. Google disregards most bad links — disavow is a last resort, not routine hygiene.

## 4. Linkable-asset strategy
- Data studies, tools, templates, and definitive guides attract links; product pages mostly don't.
- Internal-link each new asset from relevant high-authority pages.
- Digital PR: pair the asset with a news hook; pitch journalists covering the topic (`search_web` to find them).
