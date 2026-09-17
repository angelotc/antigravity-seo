---
name: seo-schema
description: Specialist skill for Schema.org structured data. Inspects, validates, and generates JSON-LD markup adhering to Google Rich Results requirements for any vertical.
---

# Schema.org Structured Data Specialist Skill

Enforces Google-compliant structured data markup to unlock rich snippets in search results. Templates below are vertical-neutral — pick the family matching the page type; the engine validates all of them.

## Priority Schema Templates

1. **Organization (every site should ship one)**:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "Organization",
     "name": "Example Co",
     "url": "https://example.com",
     "logo": "https://example.com/logo.png",
     "sameAs": [
       "https://twitter.com/exampleco",
       "https://www.linkedin.com/company/exampleco"
     ]
   }
   ```

2. **LocalBusiness (physical locations)** — engine requires `name`, `address`, `telephone`:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "LocalBusiness",
     "name": "Example Bakery",
     "url": "https://examplebakery.com",
     "telephone": "+1-555-0100",
     "address": {
       "@type": "PostalAddress",
       "streetAddress": "12 Main Street",
       "addressLocality": "Springfield",
       "addressRegion": "IL",
       "postalCode": "62701",
       "addressCountry": "US"
     },
     "openingHoursSpecification": [{
       "@type": "OpeningHoursSpecification",
       "dayOfWeek": ["Monday","Tuesday"],
       "opens": "08:00",
       "closes": "17:00"
     }]
   }
   ```

3. **Article / BlogPosting (content pages)** — engine requires `headline`, `datePublished`, `author`, `image`:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "Article",
     "headline": "How to Choose the Right Running Shoe",
     "datePublished": "2026-09-01T09:00:00Z",
     "author": {"@type": "Person", "name": "Jane Doe", "url": "https://example.com/authors/jane"},
     "image": "https://example.com/images/running-shoes.jpg",
     "publisher": {"@type": "Organization", "name": "Example Co"}
   }
   ```

4. **Product (e-commerce)** — engine requires `name`, `image`, `offers` (with `price`, `priceCurrency`):
   ```json
   {
     "@context": "https://schema.org",
     "@type": "Product",
     "name": "Trail Runner X2",
     "image": "https://example.com/trail-runner-x2.jpg",
     "brand": {"@type": "Brand", "name": "ExampleGear"},
     "offers": {
       "@type": "Offer",
       "price": "129.99",
       "priceCurrency": "USD",
       "availability": "https://schema.org/InStock"
     }
   }
   ```

5. **Event** — engine requires `name`, `startDate`, `location`:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "Event",
     "name": "City Marathon 2026",
     "startDate": "2026-11-08T07:30",
     "location": {"@type": "Place", "name": "Riverside Park", "address": "Springfield, IL"}
   }
   ```

6. **Niche listing types**: for vertical-specific pages use the matching schema.org type — `RealEstateListing` (property), `JobPosting` (jobs), `Recipe` (food), `Course` (education), `VideoObject` (video), `FAQPage` (info hubs; each `mainEntity` item pairs a `Question` with an `acceptedAnswer`). The engine validates required properties for each.

## Validation Protocol

Always run the built-in validator to confirm syntax and check required properties:
```bash
seo-engine schema <URL> --json
```
The engine validates required properties for Article, Product (incl. `offers.price`/`priceCurrency`), BreadcrumbList, FAQPage, Organization, LocalBusiness (address, telephone), Event, VideoObject, Recipe, JobPosting, Course, Person, WebSite, and RealEstateListing; flags deprecated types (HowTo, SpecialAnnouncement, ClaimReview, VehicleListing, EstimatedSalary, LearningVideo, CourseInfo) and placeholder values. Ensure zero errors before deploying.

New files written through the Antigravity plugin are additionally gated by the PostToolUse lint hook (`hooks/schema_linter.sh`, `hooks/schema_linter.ps1` on Windows), which blocks placeholder values and deprecated types at edit time.
