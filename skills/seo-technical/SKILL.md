---
name: seo-technical
description: Specialist skill for technical SEO, Core Web Vitals, HTTP headers, canonical tags, URL structure, and crawlability.
---

# Technical SEO Specialist Skill

Focuses strictly on technical infrastructure, crawlability, indexability, and site performance.

## Core Directives

1. **Verify Canonical Header & Tag Alignment**:
   * Inspect both the HTTP `Link` header and `<link rel="canonical">` tag.
   * If both exist, they **must match identically**. A mismatch confuses Googlebot and leads to unpredictable index selection.

2. **Redirect Hygiene**:
   * Standardize URL trailing slashes (e.g. `/akiya/` vs `/akiya`). Redirects should be single-hop `301 Moved Permanently` or `308 Permanent Redirect`.
   * Never chain redirects (e.g. `http -> https` then `non-www -> www`). Consolidate into a direct 1-hop rule at the reverse proxy (Nginx/Cloudflare).

3. **Core Web Vitals Thresholds (2026 Standards)**:
   * **LCP (Largest Contentful Paint)**: Good `<= 2.5s`, Needs Improvement `2.5s - 4.0s`, Poor `> 4.0s`.
   * **INP (Interaction to Next Paint)**: Good `<= 200ms`, Needs Improvement `200ms - 500ms`, Poor `> 500ms`.
   * **CLS (Cumulative Layout Shift)**: Good `<= 0.1`, Needs Improvement `0.1 - 0.25`, Poor `> 0.25`.
   * *Note*: FID was deprecated and removed by Google in September 2024. Never reference FID.

4. **Security & Protocol Headers**:
   * Enforce `Strict-Transport-Security: max-age=31536000; includeSubDomains`.
   * Ensure `X-Content-Type-Options: nosniff`.
