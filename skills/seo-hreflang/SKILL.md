---
name: seo-hreflang
description: International SEO and hreflang audit — BCP47 validation, x-default, self-reference, reciprocity, and sitemap-vs-on-page consistency. Use for multi-language or multi-region site questions.
---

# International / hreflang Audit

On-page audit:
```bash
seo-engine hreflang <url> --json
```

## Rules the engine enforces
- Valid BCP47 codes (`en`, `ja`, `pt-BR`, `zh-Hans`); `x-default` for unmatched locales.
- Every page must self-reference its own locale.
- No duplicate `hreflang` declarations.

## Verify reciprocity (the #1 hreflang bug)
Each alternate must point back. For up to 10 alternates, run:
```bash
seo-engine hreflang <alternate-url> --json
```
and confirm the original URL appears as an alternate in each response. Missing return links make the whole cluster ignored by Google.

## Consistency checks
- hreflang URLs must return 200 (not redirect) — verify with `headers <alternate-url>`.
- hreflang URLs must match their canonicals — cross-check `audit <alternate-url> --json`.
- For large sites, prefer a sitemap-based hreflang implementation (single source of truth) over per-page `<link>` tags.

## Generation guidance
When the user asks to create hreflang annotations: emit `<link rel="alternate">` for every locale **plus** self **plus** x-default, all with absolute URLs.
