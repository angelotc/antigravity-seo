---
name: seo
description: Universal SEO, GEO (Generative Engine Optimization), and technical site auditing orchestrator. Use whenever the user asks for an SEO audit, site health check, schema validation, sitemap inspection, or AI search citability review.
---

# Antigravity SEO & GEO Orchestrator

This skill orchestrates end-to-end SEO audits combining the high-speed Go `seo-engine` with Antigravity native primitives (`read_url_content`, `search_web`, `invoke_subagent`, and `<artifacts>`).

---

## 4-Phase Audit Workflow

When asked to audit a website (e.g. `https://example.com`):

### Phase 1: Transport & Crawler Health
Run the high-speed Go engine to verify the foundational HTTP and bot access layer:

1. **Header & Redirect Inspection**:
   ```bash
   seo-engine headers <URL> --json
   ```
   * Flag any non-200 status, multi-hop redirect chains, or `X-Robots-Tag: noindex`.
   * Check TTFB (warn if > 800ms).

2. **Robots.txt & AI Crawler Policy**:
   ```bash
   seo-engine robots <URL> --json
   ```
   * Verify if Googlebot, Google-Extended, and AI Search bots (OAI-SearchBot, PerplexityBot, ClaudeBot) are permitted or blocked.

3. **XML Sitemap Validation**:
   ```bash
   seo-engine sitemap <URL>/sitemap.xml --limit 15 --json
   ```
   * Check sitemap index structure, count, and verify sample URLs are returning 200 OK.

---

### Phase 2: On-Page Technical & Schema.org Audit
Run the deep DOM and structured data analyzer:

```bash
seo-engine audit <URL> --json
```

For a full six-dimension deep audit (adds images, content/E-E-A-T, hreflang), use:

```bash
seo-engine page <URL> --keyword "<primary keyword>" --json
```

Evaluate:
* **Title & Meta Description**: Length, uniqueness, keyword placement.
* **Canonical Consistency**: Is canonical present and self-referencing, or pointing elsewhere?
* **Heading Structure**: Exactly one `<h1>` matching primary query intent, logical `<h2>`/`<h3>` hierarchy.
* **Image Optimization**: Alt coverage, dimensions/CLS, formats, lazy-loading (`images` section).
* **Schema (JSON-LD)**: Verify required properties for detected types (`Organization`, `Product`, `RealEstateListing`, `LocalBusiness`, `Event`, `FAQPage`, `BreadcrumbList`).
* **Content & E-E-A-T**: Word count, byline/date signals, answer blocks for AI citability (`content` section).

---

### Phase 3: Semantic Intent & Generative Engine Optimization (GEO)
Leverage Antigravity's native tools:

1. **Page Readability & Content Signals**:
   * Call `read_url_content` on `<URL>` to inspect markdown conversion.
   * Verify **Answer-First Structure**: Does the page directly answer user search intent in the opening paragraphs?
   * Check for `/llms.txt` or `/llms-full.txt` presence.

2. **Live Competitor SERP Intelligence**:
   * Use `search_web` to inspect the top 3 ranking competitors for the target primary keyword.
   * Identify content coverage gaps (e.g. pricing tables, FAQ items, or specific regulatory details).

---

### Phase 4: Synthesis & Deliverable

Generate an interactive Markdown Artifact in this conversation's artifact directory featuring:
1. **Executive Scorecard** (Technical, Schema, GEO, and Mobile scores out of 100).
2. **Prioritized Action Items**: Categorized by severity:
   * `> [!CRITICAL]` Immediate indexing or crawl blockers (e.g. `noindex`, 404 in sitemap).
   * `> [!WARNING]` Performance, canonical, or schema deprecation issues.
   * `> [!TIP]` Citability and E-E-A-T optimization opportunities.
3. **Mermaid Site Hierarchy or Link Graph Diagram**:
   ```mermaid
   graph TD
       Home["Homepage"] --> Hub1["Category Hub"]
       Home --> Hub2["City Hub"]
       Hub1 --> Item["Detail Page"]
   ```
4. **Standalone Client Deliverable (HTML or PDF)**:
   * When an external client deliverable is requested, generate an executive HTML or PDF report directly using the engine:
     ```bash
     seo-engine report <URL> --pdf --out seo-audit-report.pdf
     ```
   * Or inspect keyless backlink graphs and first-party search analytics:
     ```bash
     seo-engine backlinks <domain> --limit 50 --json
     seo-engine gsc query sc-domain:<domain> --dimensions query,page --limit 25
     ```
