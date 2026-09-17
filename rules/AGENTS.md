# Antigravity SEO Suite — Agent Guidelines

When running SEO audits, technical checks, or content optimization:

0. **Engine Resolution (OS-agnostic)**:
   - Skills reference the engine binary as `seo-engine`. Before running, resolve it to an absolute path:
     `<plugin-root>/bin/seo-engine` (Linux/macOS) or `<plugin-root>\bin\seo-engine.exe` (Windows).
   - `<plugin-root>` is this plugin's directory — typically `~/.gemini/config/plugins/antigravity-seo`
     after installation, or the repository checkout during development.
   - All engine paths, data directories (`seo-engine doctor` shows the location), and output files use
     the host OS separators; never assume `/`-prefixed Unix paths.

1. **Prefer `seo-engine` for Network & Transport**:
   - Always run `seo-engine headers <url>` to inspect status codes, 301/302 hops, `X-Robots-Tag`, and TTFB.
   - Run `seo-engine sitemap <url>` for sitemap checks rather than manually fetching XML.
   - Run `seo-engine robots <url>` to verify search bot and AI crawler permissions (Google-Extended, GPTBot, ClaudeBot).

2. **Use Native `read_url_content` for Semantics**:
   - Use `read_url_content` for zero-overhead markdown conversion to evaluate keyword targeting, topical depth, and internal linking structure.

3. **Use Native `search_web` for SERP Intelligence**:
   - Query live Google search results with `search_web` to inspect who ranks for target queries, assess PAA (People Also Ask) questions, and identify competitor content gaps.

4. **Multi-Agent Swarm Tiering**:
   - When fanning out across multiple pages (e.g. category hubs or regional listing pages), use `invoke_subagent` with `Model: "flash_lite"` or `"flash"` for high-speed parallel crawls.
   - Reserve `"pro"` or reasoning models for synthesis, E-E-A-T evaluation, and Generative Engine Optimization (GEO) scoring.

5. **Deliver Structured Artifacts**:
   - Present full site audit findings as UI Artifacts in the auxiliary panel with GitHub alert banners (`> [!CRITICAL]`, `> [!WARNING]`) and Mermaid site architecture diagrams.
