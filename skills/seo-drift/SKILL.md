---
name: seo-drift
description: Continuous SEO drift monitoring, regression detection, and cross-session baseline tracking using Antigravity schedule and Memex.
---

# Continuous SEO Drift & Regression Monitoring

Detects accidental SEO regressions (e.g. accidental `noindex` deployments, broken canonicals, 404s, or schema drops) over time.

---

## 1. Autonomous Background Monitoring via `schedule`

To monitor a production site continuously, set up a recurring cron task using Antigravity's native `schedule` tool:

```json
{
  "CronExpression": "0 2 * * *",
  "Prompt": "Run a fast SEO health check on https://nipponhomes.com key pages (home, /akiya, /tokyo). Verify: 1) status is 200, 2) canonical matches self, 3) noindex is absent, 4) schema JSON-LD is valid. Alert in conversation if any regression is detected."
}
```

The agent will awaken each night, run `/apps/antigravity-seo/bin/seo-engine headers <url>` and report only if regressions are identified.

---

## 2. Cross-Session Baseline Tracking with Memex

Before starting an audit, check if a previous audit baseline exists using Memex:

```bash
memex search "<domain> SEO audit" --limit 3 --unique-session
```

### What to Compare Against Baseline:
* Did the Technical Score or Schema Score change?
* Were previously resolved critical issues reintroduced in a recent code commit?
* Did new redirect hops get added to primary navigation URLs?
