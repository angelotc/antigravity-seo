---
name: seo-images
description: Image SEO audit and optimization guidance — alt text quality, dimensions for CLS, modern formats, lazy loading, and file naming. Use when the user asks about image optimization or the page audit flags image issues.
---

# Image SEO

Run the engine audit:
```bash
seo-engine images <url> --json
```

## Fix priority
1. **Missing alt** (accessibility + image search): describe function, not appearance; empty `alt=""` only for decorative images.
2. **Missing width/height**: leading cause of CLS failures; always set explicit dimensions or aspect-ratio.
3. **Legacy formats**: convert jpg/png to WebP (typically 25–35% smaller) or AVIF (50%+); keep fallback via `<picture>`.
4. **No lazy-loading**: add `loading="lazy"` to below-fold images; the LCP image must NOT be lazy.
5. **Oversized data URIs**: >10KB inline images bloat HTML; link files instead.
6. **Insecure http:// images**: mixed content blocks padlock and triggers browser warnings.

## Beyond the engine (use `read_url_content` + judgment)
- Filenames: `handmade-cedar-desk-oak-finish.webp` beats `IMG_0042.jpg`.
- LCP image: preload it (`<link rel="preload" as="image">`) and consider `fetchpriority="high"`.
- AI-generated product images should carry IPTC `TrainedAlgorithmicMedia` metadata for disclosure compliance.
