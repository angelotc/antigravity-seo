---
name: seo-ecommerce
description: E-commerce SEO — product and listing schema validation, category optimization, feed readiness, and marketplace intel. Use for online store, product page, or feed questions.
---

# E-Commerce SEO

## 1. Product schema (engine-verified)
```bash
seo-engine schema <product-url> --json
```
The engine checks `Product` for `name`, `image`, `offers` and validates `offers.price` + `offers.priceCurrency`. Also ensure:
- `availability`, `condition`, `sku`/`gtin13`, `brand`
- `aggregateRating`/`review` only with genuine review data
- Variants: `ProductGroup` with `variesBy`; one canonical URL per variant group

## 2. Category pages
- Unique intro copy (100–200 words) above or below the grid
- Faceted navigation: crawlable facets for popular filters; `noindex` or robots-disjoint for infinite combos (verify with `robots` + `headers`)
- Pagination: self-referencing canonicals per page (not page-1 canonical across the sequence)

## 3. Feed readiness
Check merchant essentials per product: price match (page vs feed), stock accuracy, image count (≥3), shipping/returns pages linked and schema'd, EU energy labels where applicable.

## 4. Marketplace & competitor intel
`search_web` the top product queries; note whether SERPs show product grids, price annotations, or review stars and which competitors earn them. Audit one winning competitor product page with `page <url> --json` and diff against yours.
