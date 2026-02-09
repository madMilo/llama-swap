# UI migration plan: Svelte → Haml + htmz + Topcoat

## Goals
- Replace the compiled Svelte UI with server-rendered Haml templates.
- Use **htmz** for dynamic UI updates (no SPA build step).
- Adopt **Topcoat** components for consistent styling.
- Ensure core pages work **without JavaScript** (progressive enhancement).

## Scope
- Replace the UI build pipeline under `ui-svelte/` with server-side templates.
- Serve Haml-rendered HTML from the Go backend.
- Add htmz endpoints for partial updates.
- Keep all endpoints accessible with plain HTML (no JS requirement).

## Architecture
### 1) Templates
- Create a new `ui/` or `templates/` directory:
  - `layouts/` base layout(s)
  - `pages/` full page views
  - `partials/` fragments for htmz updates
- Adopt a **Go Haml library** for template rendering.

### 2) Topcoat styling
- Include Topcoat CSS as a static asset (local copy, no CDN dependency).
- Replace existing UI components with Topcoat equivalents:
  - Buttons, inputs, tables, tabs, nav
  - Status badges / alerts

### 3) htmz for dynamic updates
- Add endpoints that return HTML fragments for htmz swaps:
  - Model list
  - Running processes
  - Logs stream / summary
  - Metrics snapshots
- htmz-enabled pages should degrade gracefully:
  - Base page renders full content
  - htmz replaces targeted sections

### 4) No-JS mode
- Ensure every page renders server-side HTML only.
- Use normal links and forms that submit via HTTP.
- htmz and any optional JS should only enhance.

### 5) htmz script delivery
- Ship a **small htmz client script** as a static asset.
- Pages should work without it; enable it for progressive enhancement.

## Migration steps
1. **Inventory Svelte routes/components**
   - Map existing pages to Haml templates.
2. **Introduce template renderer**
   - Add template loader and rendering pipeline in Go using a Haml library.
3. **Build base layout + navigation**
   - Include Topcoat CSS.
4. **Rebuild key pages**
   - `/ui/models`, `/ui/running`, `/ui/logs`
5. **Add htmz partial endpoints**
   - Use `Accept: text/html` fragments.
6. **Remove Svelte build**
   - Delete `ui-svelte/` build steps and bundling.

## Non-goals
- No client-side router.
- No bundler (Vite/Svelte/etc.).
- No mandatory JS.

## Open questions
- Whether to use a Go Haml library vs. a minimal custom template syntax.
- htmz: whether to ship a small script or use pure HTML with periodic refresh endpoints.
