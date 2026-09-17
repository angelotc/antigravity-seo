---
name: seo-geo
description: Specialist skill for Generative Engine Optimization (GEO) and AI Search citability. Evaluates how well content is synthesized and cited by Google AI Overviews, ChatGPT Search, and Perplexity.
---

# Generative Engine Optimization (GEO) Specialist Skill

Optimizes content for generative discovery, AI answer engines, and LLM-based search crawlers.

## Key Principles per Google's AI Optimization Guide

1. **Answer-First Structure (Inverted Pyramid)**:
   * AI search engines scan for immediate, unambiguous factual answers.
   * Provide the direct answer to the heading's implied question within the first 1–2 sentences before expanding into nuance, background, or commentary.

2. **Entity Salience & Unambiguous Naming**:
   * Refer to entities (locations, companies, products, regulations) by their canonical full names rather than pronouns ("it", "they").
   * Connect entities to their Wikidata, Wikipedia, or official definitions in schema (`sameAs`).

3. **Information Density & Fact Extraction**:
   * Use bulleted definitions, comparison tables, and structured data summaries.
   * High information density scores correlate directly with citation rates in Google AI Overviews and Perplexity.

4. **`llms.txt` and `llms-full.txt` Specification**:
   * Audit with the engine: `seo-engine llms <url> --json` — checks existence, H1 title, section count, and markdown link coverage for both `/llms.txt` and `/llms-full.txt`.
   * A well-formed `/llms.txt` gives AI agents a clean, markdown-indexed map of core documentation, reducing hallucinated summaries. Treat it as a discovery aid, not a citation lever.

5. **AI Crawler Policies**:
   * Check `robots.txt` using `seo-engine robots <url>`.
   * Note: Blocking `Google-Extended` protects proprietary training data for Gemini/Vertex without impacting Google Search indexing. Blocking `Googlebot` or `OAI-SearchBot` blocks search visibility.
