---
name: seo-cluster
description: SERP-based semantic keyword clustering and topical authority mapping. Use for keyword research, clustering, topic architecture, or hub-and-spoke planning.
---

# Keyword Clustering

Group keywords by SERP similarity, not string similarity — Google co-ranking two queries means one page can target both.

## Method
1. **Seed collection**: start from the money keywords; expand with `search_web` for each seed (PAA boxes, related searches, autocomplete variants). Target 50–200 raw queries.
2. **SERP sampling**: for each keyword, capture the top-10 ranking URLs via `search_web`.
3. **Cluster rule**: keywords sharing ≥4 of top-10 URLs (≥40% overlap) belong to one cluster — one page serves them. Overlap ≤3 → separate pages.
4. **Intent split**: within a cluster, never mix intents (informational vs transactional vs navigational) even if SERPs overlap.

## Output artifact
- **Cluster map**: hub page per cluster; spokes per sub-intent.
- Per cluster: primary keyword (highest volume × best fit), secondary keywords, SERP features present (PAA, featured snippet → answer-block opportunity, map pack → local intent).
- **Cannibalization check**: any existing URLs ranking for multiple clusters need consolidation — run `sitemap` + `audit` to inventory current pages.

## Validation
For each planned hub, run `read_url_content` on the current top-3 results and list covered subtopics vs gaps — the gaps define the hub's competitive edge.
