# Antigravity Marketplace & Registry Submission Guide

This guide documents the submission package and instructions for listing `antigravity-seo` on Google Antigravity's plugin catalogs, "Build with Google", and community plugin registries.

---

## 1. Plugin Metadata Summary

| Field | Value |
|---|---|
| **Plugin ID** | `antigravity-seo` |
| **Display Name** | Antigravity SEO & GEO Suite |
| **Version** | `2.1.0` |
| **License** | MIT |
| **Repository** | `https://github.com/angelotc/antigravity-seo` |
| **Manifest** | [`plugin.json`](../plugin.json) |
| **Marketplace Spec** | [`marketplace.json`](../marketplace.json) |

---

## 2. Validation Status

The plugin is verified using the official Antigravity CLI validator:

```bash
agy plugin validate .
```

```text
  [ok]    .
          ✔ skills      : 21 processed
          - agents      : skipped (not found)
          - commands    : skipped (not found)
          - mcpServers  : skipped (not found)
          ✔ hooks       : 1 processed
```

---

## 3. Submission Channels

### A. Google Antigravity / "Build with Google"
When Google opens public submissions or developer partner onboarding for the Antigravity Plugin Catalog:
1. Provide the repository URL: `https://github.com/angelotc/antigravity-seo.git`
2. Point to the verified [`plugin.json`](../plugin.json) and [`marketplace.json`](../marketplace.json).
3. Specify the runtime contract:
   - Small pure-Go binary (deps: `golang.org/x/net`, `golang.org/x/text`, `github.com/go-pdf/fpdf`); no Python/Playwright runtime or external PDF renderer.
   - Compliant PostToolUse schema linting hook (`hooks.json`), bash-wired by default — see the README's "Hooks" section for the Windows/PowerShell caveat.
   - Native integration with Antigravity tools (`search_web`, `read_url_content`).

### B. Community Registries (`awesome-antigravity`, etc.)
For community-maintained plugin lists and registries:
- **One-line Install Command**:
  ```bash
  curl -fsSL https://raw.githubusercontent.com/angelotc/antigravity-seo/main/install.sh | bash
  ```
- **Workspace-level Install Command**:
  ```bash
  git clone https://github.com/angelotc/antigravity-seo.git .agents/plugins/antigravity-seo
  ```

---

## 4. Release Artifacts

Multi-platform release binaries are automatically compiled and attached to GitHub Releases via GitHub Actions on every semantic tag (`v*.*.*`):

- `seo-engine-linux-amd64`
- `seo-engine-linux-arm64`
- `seo-engine-darwin-amd64`
- `seo-engine-darwin-arm64`
- `seo-engine-windows-amd64.exe`
