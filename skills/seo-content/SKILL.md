---
name: seo-content
description: Specialist skill for content quality, E-E-A-T evaluation, search intent matching, and topical cluster architecture.
---

# Content Quality & E-E-A-T Specialist Skill

Audits content against Google's Quality Rater Guidelines (QRG) focusing on Experience, Expertise, Authoritativeness, and Trustworthiness (E-E-A-T).

## Engine-Assisted Content Audit

Run the quantitative layer first, then apply the checklist below:
```bash
seo-engine content <url> --keyword "<target phrase>" --json
```
It measures word count, heading hierarchy (skipped levels), author byline and date signals, answer blocks (40–80 words) and AI-citation passages (130–180 words), plus keyword density/placement.

## E-E-A-T Assessment Checklist

1. **Experience (First-Hand Knowledge)**:
   * Does the content feature real user experiences, original photos, actual transaction breakdowns, or on-the-ground case studies?
   * Avoid generic AI summaries that lack firsthand perspective.

2. **Expertise (Credential Proof)**:
   * Are author bylines present with verifiable biographical details and credentials relevant to the topic (e.g. licensed professional, certified practitioner, long-time local operator)?

3. **Authoritativeness (Topical Depth)**:
   * Does the site cover the topic through a complete **Hub-and-Spoke** cluster architecture?
   * *Example*: a main Buying Guide (Hub) supported by category/regional guides (Spokes: by product line, by city, by use case) interconnected via contextual internal links.

4. **Trustworthiness (Foundational Safety)**:
   * Clear company disclosures (Legal business entity, About Us page, Contact info, Privacy Policy, Terms of Service).
   * For YMYL topics (finance, health, legal, property): transparent disclosures on fees, risks, and jurisdiction-specific regulations.
