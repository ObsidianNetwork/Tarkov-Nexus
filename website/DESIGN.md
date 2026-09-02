# Tarkov Nexus Website — Design System

The implementation contract for the marketing site (`website/`). Every color,
font size, spacing value, shadow, and animation in the site traces back to a
token defined here. Change the token here, not the raw value in a component.

## Identity

Dark, tactical, tracker-inspired. Purple primary inherited from the app
branding, neon-green as the "live / success" signal, amber reserved for
pre-release (beta) and warning surfaces. Gradients are used sparingly as
material (text fills, glows), never as the base surface.

## 1. Tokens

### Color

| Token | Value | Use |
|---|---|---|
| `--primary-purple` | `#5865F2` | Primary actions, links, focus accents |
| `--dark-purple` | `#4752C4` | Gradient partner of primary |
| `--light-purple` | `#7289DA` | Hover accents |
| `--electric-purple` | `#8B5FBF` | Secondary gradient partner, beta accent |
| `--neon-green` | `#00D4AA` | Live data, success, version numbers |
| `--amber` | `#F0A020` | Beta channel, warnings, notice |
| `--bg-darker` | `#06080B` | Page base |
| `--bg-dark` | `#0C0E14` | Alternating section background |
| `--bg-card` | `#1A1D26` | Card surface |
| `--bg-hover` | `#232631` | Hover surface |
| `--text-primary` | `#FFFFFF` | Headings, body |
| `--text-secondary` | `#B9BBBE` | Paragraphs, descriptions |
| `--text-muted` | `#72767D` | Labels, footnotes |
| `--border-color` | `#2A2D38` | Hairlines, card borders |

All grays are cool-tinted. Never introduce pure `#000` or warm grays.

### Typography

| Role | Font | Notes |
|---|---|---|
| Display / headings | Space Grotesk (600, 700) | Tight tracking (`-0.02em`+), `text-wrap: balance` |
| Body | Inter (400, 500, 600, 700) | Max reading width ~65ch, line-height 1.6 |
| Data / labels | JetBrains Mono (400, 600) | Eyebrows, badges, version strings, step numbers, counters — always `tabular-nums` where numeric |

Type scale (fluid): hero `clamp(2.5rem, 6vw, 4.25rem)`; h2 `clamp(1.9rem, 3.5vw, 2.6rem)`; card h3 `1.2rem`; body `1rem`; small `0.875rem`.

### Spacing & radius

Section rhythm `7rem` desktop / `4rem` mobile. Card padding `2rem`–`3rem`.
Radius scale: `--radius-sm: 8px`, `--radius-md: 12px`, `--radius-lg: 20px`,
`--radius-full: 999px`. Inner elements are tighter than their containers.

### Elevation & glow

Shadows are tinted (purple-black, never pure black):

- `--shadow-card: 0 1px 0 rgba(255,255,255,0.04) inset, 0 8px 24px -12px rgba(3,5,10,0.8)`
- `--shadow-pop: 0 24px 48px -16px rgba(3,5,10,0.9)`
- `--glow-primary: 0 0 24px rgba(88,101,242,0.45)` (buttons, focus moments)
- `--glow-green: 0 0 20px rgba(0,212,170,0.35)` (live signals)

### Z-index scale

`bg: -1` · `content: auto` · `header: 100` · `notice: 200` · `fab: 150` ·
notice renders above the FAB when both are visible (notice bottom offset
clears the FAB).

### Motion

- Durations: `--t-fast: 150ms`, `--t-med: 250ms`, `--t-slow: 400ms`
- Easing: `--ease-out: cubic-bezier(0.22, 1, 0.36, 1)`; press feedback uses ease-out
- GPU-composited only: `transform`, `opacity`, `filter`
- Scroll reveal: `.reveal` starts `opacity:0; translateY(20px)`, becomes
  visible via IntersectionObserver; stagger with `--reveal-delay` in `ms`
- Every animated element honors `prefers-reduced-motion: reduce` (no
  animation, final state immediately)

## 2. Primitives

- **`.btn-primary`** — purple gradient fill, glow shadow, hover lift `-2px` +
  stronger glow, press `scale(0.98)`.
- **`.btn-secondary`** — card surface + border; hover: border → primary.
- **`.btn-beta`** — amber-outline variant for beta channel actions; amber tint
  glow on hover. Never used for stable actions.
- **`.card`** — `--bg-card`, hairline border, inset top highlight, tinted
  shadow; hover: border brightens + lift `-4px`.
- **`.eyebrow`** — JetBrains Mono uppercase label, letter-spacing `0.14em`,
  muted or green, used above section h2s and in the hero.
- **`.section-header`** — eyebrow + h2 + lede, centered or left-aligned per
  section; left-aligned for asymmetric sections.
- **`.chip`** — small pill with mono text (version, status, channel).
- **`.reveal`** — scroll-reveal utility (see Motion).

## 3. Layout grammar

- Container: `max-width: 1200px`, `padding-inline: 1.5rem`.
- Hero: `min-height: 100svh` (never `100vh`), 2-col grid collapsing at 1024px.
- Features: **bento grid** — Real-Time Position, Party System, and Quest
  Tracking span 2 columns each, filling three full rows; collapses to 2-col
  (1024px, wides span full width) then 1-col (640px).
- Sections alternate `--bg-darker` / `--bg-dark` with hairline separation.
- Fixed background: two radial purple washes + SVG grain overlay (noise breaks
  digital flatness; always `pointer-events: none`, behind content).

## 4. Accessibility constraints

- Visible `:focus-visible` ring on every interactive element (2px primary + offset).
- Skip-to-content link as the first focusable element.
- Icon-only controls require `aria-label`.
- Live regions (`aria-live`, `aria-busy`) on async counters.
- Color is never the only signal (beta uses label + color).
- Contrast: secondary text `#B9BBBE` on `#1A1D26` ≈ 8.9:1 — never darken body
  text below this.

## 5. Content rules

- Sentence case everywhere; no exclamation marks in product copy.
- No "elevate / seamless / unleash / next-gen" clichés.
- Version strings and download counts always mono + tabular.
- The site stays a single page; new content earns a section only when it
  changes the visitor's decision path (hook → explain → prove → convert → retain).

## 6. Analytics contract

Events are per-surface so the GA4 dashboard answers "what is happening, where":

| Event | Trigger | Params |
|---|---|---|
| `download_click` | Any release-asset link | `channel` (stable/beta), `location` (section id), `file_name`, `link_url` |
| `nav_click` | Header/footer nav links | `nav_item`, `destination` |
| `faq_open` | `<details>` opened | `question` |
| `feedback_click` | Any feedback/issues entry point | `entry_point` (fab/header/footer/beta-card) |
| `section_view` | Section ≥40% visible, once | `section_id` |

Implement links with `data-analytics` + typed data attributes; the tracker in
`Analytics.astro` is the only place that calls `gtag`.

## 7. Accepted debt

- `aggregateRating` in JSON-LD is a self-assigned 5/1 rating — SEO risk if
  flagged; kept from the original site.
- Beta download links to the live `v3.3.4-beta.1` Windows-x64 asset; update the
  `BETA` constant in `Download.astro` (url + version) whenever a new beta ships.
- FAQ is accordion-style; acceptable for 10 items, revisit if it grows.
