---
name: seo-content-brief
description: Generate a complete content brief for a target topic — keywords, outline, internal links, schema, and E-E-A-T requirements. Use before writing any new article or landing page.
---

# Content Brief Generator

Input: target topic or keyword. Output: a writer-ready brief.

## 1. Keyword set
- Primary: the head term with best volume/intent fit
- Secondary: 5–15 supporting terms from `search_web` (PAA, related searches, autocomplete)
- Filter: drop anything outside the page intent

## 2. SERP contract (what the page must contain)
`read_url_content` on the current top-3 results: list their H2 structures, formats (tables, tools, videos), and words on page. The brief must match-or-beat on depth and add one unique element.

## 3. Outline
```
H1: {{primary keyword, natural phrasing}}
  Intro: 40-80 word direct answer block (engine `content` checks this)
  H2: {{topical section}} — target secondary keyword
    ...
  H2: FAQ — 3-5 PAA questions verbatim
  H2: Next steps / CTA
```

## 4. On-page requirements
- Title tag (≤60 chars, primary keyword front-loaded)
- Meta description (70–160 chars, includes benefit + keyword)
- Schema: Article (+FAQPage only if eligible); author byline and publish date (E-E-A-T signals the engine verifies)
- Images: ≥1 unique visual, alt text specified per image
- Word count target from SERP median +15%

## 5. Internal linking
List 3–5 existing site pages to link from/to (find via `sitemap`); anchor text: the target page's primary keyword variants, never "click here".

## 6. Post-publish verification
```bash
seo-engine page <new-url> --keyword "<primary>" --json
seo-engine drift baseline <new-url>
```
