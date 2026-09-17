---
name: seo-drift
description: Continuous SEO drift monitoring, regression detection, and cross-session baseline tracking using Antigravity schedule and Memex.
---

# Continuous SEO Drift & Regression Monitoring

Detects accidental SEO regressions (e.g. accidental `noindex` deployments, broken canonicals, 404s, or schema drops) over time.

---

## 1. Snapshot Drift Store (engine-native)

The engine stores page snapshots (title, meta, canonical, H1, schema types, word count, status, TTFB, content hash) in its data directory — see `seo-engine doctor` for the OS-specific location.

```bash
seo-engine drift baseline <url>      # capture a snapshot
seo-engine drift compare <url>       # field-level diff vs latest baseline
seo-engine drift history <url>       # list all snapshots
```

Typical loop: baseline after every intentional deploy, compare before/after risky changes. The compare report flags any field drift — status drops, canonical changes, schema type removals, or TTFB regressions.

---

## 2. Autonomous Background Monitoring via `schedule`

To monitor a production site continuously, set up a recurring cron task using Antigravity's native `schedule` tool:

```json
{
  "CronExpression": "0 2 * * *",
  "Prompt": "Run `seo-engine drift compare` on the key pages of https://example.com (home, /pricing, /blog); if no baseline exists, run `drift baseline` first. Alert in conversation if any regression is detected (status != 200, canonical change, noindex, schema drop)."
}
```

The agent will awaken each night, run the drift compare and report only if regressions are identified.

---

## 3. Cross-Session Baseline Tracking with Memex

For cross-conversation audit history beyond page snapshots, check prior audit sessions using Memex:

```bash
memex search "<domain> SEO audit" --limit 3 --unique-session
```
