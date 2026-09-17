---
name: seo-competitor-pages
description: Head-to-head competitor page comparison — identify why competing pages outrank yours and what content/technical gaps to close. Use when the user asks why a competitor ranks higher.
---

# Competitor Page Comparison

## 1. Gather the contenders
`search_web` the target keyword; take the top 3 URLs plus the user's page.

## 2. Audit all four identically
```bash
seo-engine page <url> --keyword "<keyword>" --json
```
(One subagent per URL via `invoke_subagent` when parallel.)

## 3. Diff table (the deliverable)
| Signal | You | Comp A | Comp B | Comp C |
|---|---|---|---|---|
| content score / word count | | | | |
| answer blocks / keyword in lede | | | | |
| schema types + validity | | | | |
| image alt coverage | | | | |
| TTFB | | | | |
| internal/external link count | | | | |

## 4. Qualitative pass (`read_url_content` on each)
- Entities covered by competitors but missing on your page
- Freshness signals (dates, updated figures)
- Unique value: what does each competitor offer (calculator, table, video, citations) and what can you exceed them on

## 5. Verdict
Rank the top 3 gaps by (signal deficit × implementation cost). Never recommend rewriting everything — pick the 2–3 load-bearing deltas.
