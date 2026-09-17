---
name: seo-schema
description: Specialist skill for Schema.org structured data. Inspects, validates, and generates JSON-LD markup adhering to Google Rich Results requirements.
---

# Schema.org Structured Data Specialist Skill

Enforces Google-compliant structured data markup to unlock rich snippets in search results.

## Priority Schema Templates

1. **Organization / RealEstateAgent**:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "RealEstateAgent",
     "name": "Nipponhomes",
     "url": "https://nipponhomes.com",
     "logo": "https://nipponhomes.com/logo.png",
     "sameAs": [
       "https://twitter.com/nipponhomes"
     ]
   }
   ```

2. **RealEstateListing / SingleFamilyResidence**:
   ```json
   {
     "@context": "https://schema.org",
     "@type": "SingleFamilyResidence",
     "name": "Traditional Japanese Akiya in Kyoto",
     "description": "Renovated 3-bedroom wooden house with garden.",
     "address": {
       "@type": "PostalAddress",
       "addressLocality": "Kyoto",
       "addressRegion": "Kyoto Prefecture",
       "addressCountry": "JP"
     },
     "offers": {
       "@type": "Offer",
       "price": "15000000",
       "priceCurrency": "JPY"
     }
   }
   ```

3. **FAQPage (For Informational Hubs)**:
   * Each item in `mainEntity` must pair a `Question` with an `acceptedAnswer` of type `Answer`.

## Validation Protocol

Always run the built-in validator to confirm syntax and check required properties:
```bash
/apps/antigravity-seo/bin/seo-engine schema <URL> --json
```
Ensure zero syntax errors and no missing mandatory fields before deploying structured data.
