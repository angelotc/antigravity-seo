---
name: seo-sxo
description: Search experience optimization — page-type mapping, user stories, persona-driven intent alignment, and wireframe-level SEO recommendations. Use when designing or restructuring pages around user intent.
---

# SXO: Search Experience Optimization

Bridges keyword research and UX: rank *and* satisfy.

## 1. Page-type mapping
For a target keyword cluster, identify the dominant SERP page type (tool, comparison, directory, guide, product, calculator). Match it — fighting the dominant type wastes crawl equity. Verify with `search_web` on the top 5 results.

## 2. Persona → user story extraction
For each persona touching the page:
```
As a {{persona}},
when searching "{{keyword}}",
I want {{job-to-be-done}}
so that {{outcome}},
and I'll trust the page if it shows {{trust signals: authorship, data, pricing transparency, recency}}.
```

## 3. Above-the-fold contract
The first viewport must answer: what is this, who is it for, is it current, and what do I do next. Check with the engine:
```bash
seo-engine content <url> --keyword "<keyword>" --json
```
`keyword_in_lede: false` or `answer_blocks: 0` means the page buries the answer.

## 4. Wireframe recommendations
Output a section-ordered wireframe: answer block → proof (data/reviews) → mechanism/how it works → objections/FAQ → CTA. Tie each section to a user story and a target keyword from the cluster.
