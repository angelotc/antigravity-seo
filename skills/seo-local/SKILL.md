---
name: seo-local
description: Local SEO audit — Google Business Profile signals, NAP consistency, local schema (LocalBusiness), citations, and review presence. Use for local business or "near me" visibility questions.
---

# Local SEO Audit

## 1. LocalBusiness schema (engine-verified)
```bash
seo-engine schema <url> --json
```
The engine validates `LocalBusiness` for `name`, `address`, `telephone`. Also verify manually:
- `address` matches GBP exactly (street-level, same formatting)
- `geo` coordinates, `openingHoursSpecification`, `aggregateRating` (only if real reviews exist)
- `url` points to the location landing page, not the homepage

## 2. NAP consistency (Name, Address, Phone)
Use `search_web` for `"{{business name}}" "{{phone}}"` and `"{{business name}}" "{{address}}"` to surface citations. Compare formatting across the top 15 results (GBP, Bing Places, Apple Maps, industry directories, social profiles). Flag every deviation — inconsistent NAP splits ranking signals across duplicate entities.

## 3. Google Business Profile signals
- Primary category matches the highest-intent keyword; secondary categories for breadth.
- Website link → the city/service landing page, not homepage.
- Weekly posts, Q&A seeded, photos (< 90 days old), review velocity.
- Retired features (chat, `.business.site` URLs) must not be referenced anywhere.

## 4. Local content
One landing page per city×service combination, each with unique local proof (cases, reviews, landmarks). Guard against doorway pages — see the `seo-programmatic` skill for the 30/50 quality gates.

## 5. Reviews
Respond to all reviews (esp. negatives); include service keywords naturally in responses. Never incentivize reviews (policy violation).
