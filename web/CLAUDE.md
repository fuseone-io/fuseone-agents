# FuseOne Agents — Web Guidelines

Console and Studio for the agent platform. React 19 + Vite + React Router v7 +
strict TypeScript, built to static assets and embedded in the Go binary via
`go:embed` behind the `embedui` build tag.

Core (Go) rules are in [../CLAUDE.md](../CLAUDE.md). These rules win inside `web/`.

**Language:** code, comments and identifiers in English. Every user-visible
string goes through i18n — pt-BR and en-US, always in parity. A literal in JSX
is a bug.

---

## Rule zero: never hand-write a UI control

**Every interactive control comes from `@/components/ui`.** If the component
does not exist yet, add it — `npx shadcn@latest add <component>` — before
writing a single line of markup.

This is not a style preference. A hand-rolled `<select>`, a `<div>` with an
`onClick`, a home-made tabs strip: each one silently drops keyboard
navigation, focus management, ARIA wiring, screen-reader announcements and
RTL handling that Radix already gets right. Those defects do not show up in
review or in a screenshot — they show up when someone who navigates by
keyboard cannot approve a pending action.

### Forbidden markup and its replacement

| Never write | Always use |
|---|---|
| `<button>`, `<div onClick>` | `Button` |
| `<select>`, `<option>` | `Select` (or `Combobox` built from `Command` + `Popover` when searchable) |
| `<input>` | `Input`, `Textarea`, `InputOTP` |
| `<input type="checkbox">` | `Checkbox` (or `Switch` for on/off state) |
| `<input type="radio">` | `RadioGroup` |
| `<input type="date">` | `Calendar` + `Popover` |
| `<dialog>`, a modal built from `position: fixed` | `Dialog`, `AlertDialog` for destructive confirmation, `Sheet` for a side panel |
| A tabs strip made of buttons | `Tabs` |
| `title="..."` as a tooltip | `Tooltip` |
| A `<span>` styled as a chip or tag | `Badge` |
| `<table>` written by hand | `Table` (plus TanStack Table when sortable, filterable or paginated) |
| A dropdown made of absolutely positioned divs | `DropdownMenu`, `Popover`, `ContextMenu` |
| A hand-made accordion with `useState` | `Accordion`, `Collapsible` |
| `<hr>`, a `border-t` divider div | `Separator` |
| `<progress>`, a bar div | `Progress` |
| `alert()`, a toast written from scratch | `sonner` toast |
| A spinner div, "Loading..." text | `Skeleton` |
| `<form>` with manual state | `Form` + react-hook-form + zod |
| A scroll container with custom scrollbars | `ScrollArea` |
| An avatar `<img>` with a fallback ternary | `Avatar` |
| A keyboard-shortcut palette built by hand | `Command` |

Plain `<div>`, `<span>`, `<p>`, `<ul>`, `<li>`, `<section>`, `<h1>`–`<h6>` for
layout and text are fine and expected. The rule is about **controls**, not
about every element.

### When shadcn has no component for it

1. Check the shadcn registry and the blocks first — it is probably there.
2. If it genuinely is not, build it in `src/components/ui/` **on Radix
   primitives**, following the conventions of the neighbouring files: `cva`
   variants, `cn()` for class composition, `data-slot` attributes, forwarded
   refs, no hard-coded colours.
3. Never build a one-off control inside a feature folder.

### Do not edit generated primitives casually

Files in `src/components/ui/` come from the CLI. Change one only for a real
product need, and note the reason in a comment at the top so the next
`shadcn add` diff is resolvable.

---

## Panel shell

`AppShell` is the **`sidebar-07`** block adapted to the design system, and is
the base layout for both Console and Studio. Do not invent a second shell.

- Sidebar 248px, collapsing to 48px, one border on its right edge. Navigation
  is grouped by job — Operar, Governar — in `components/layout/nav.ts`.
- Header 52px, with a **`1px` bottom border** running the width of the main
  region.
- Content is **flush**: it sits directly on `background`, `p-6`, `gap-6`, with
  no border, radius, margin or shadow on the container.
- **One level of lift, not two.** Elevation belongs to the cards inside a page
  (`rounded-xl` + `shadow-sm`), never to the page container. A container that
  also lifts makes every card inside it mean less.
- The header's rule and the flush content are one decision: the rule separates
  chrome from content because no floating panel is doing that job. Do not
  reintroduce either half alone.
- **Every screen leads with an icon tile**: 34px, `rounded-md`, hairline
  border, `muted` fill, holding a 17px lucide icon. The icon comes from
  `PAGE_ICONS` in `components/layout/nav.ts` so a screen and its navigation
  entry cannot show two different symbols for the same thing. In edit modes it
  precedes the record's identifier and never replaces it.
- **Only routes that exist appear in the navigation.** An entry pointing at a
  screen nobody built teaches people the console is unreliable.

---

## Layout

```
src/
  components/ui/          shadcn primitives. CLI-owned
  components/layout/      AppSidebar, Header, PageShell
  components/shared/      Reused across two or more features
  features/<domain>/      Everything for one domain: components, hooks, api, types
  hooks/                  Cross-cutting hooks only
  lib/                    api client, formatters, validators — named by domain
  i18n/                   pt-BR.json, en-US.json
  routes/                 Route definitions
```

- Never create `utils.ts` or `helpers.ts`. Name by domain: `cost-format.ts`,
  `run-status.ts`.
- One component per file. Exception: an internal subcomponent under 30 lines
  used only by its parent.
- Types live beside the feature that owns them. Promote to `src/types/` only
  when three or more features share them.
- Imports through the `@/` alias, never `../`.

---

## Size limits

| Unit | Limit |
|---|---|
| Component | 150 lines including JSX |
| Hook | 80 lines |
| Route file | 100 lines — heavy logic goes to hooks |
| Props | 5 — beyond that, composition or a config object |
| JSX nesting | 4 levels |
| Tailwind classes on one element | ~6 |

At 120 lines in a component, stop and extract.

---

## State and data

- **TanStack Query** for everything from the server. Never `useEffect` +
  `fetch`. Query keys are typed and centralised per feature.
- **Zustand** for client state that outlives a component. No Redux, no Context
  API as a global store.
- **react-hook-form + zod** for every form. The zod schema is the single
  source of validation; do not restate rules in JSX.
- Live run updates arrive over **SSE**, consumed through a dedicated hook that
  reconciles into the Query cache. Components never open an EventSource.
- A hook that fetches returns at least `{ data, isLoading, error }`.

---

## The run diagram

- **XYFlow** to render, **elkjs** to lay out.
- The diagram is **read-only and generated** from the agent specification.
  There is no drag-and-drop authoring (PRD N5).
- Layout must be **deterministic**: the same specification renders identically
  every time. It appears on approval screens and in audit records, so a
  diagram that reshuffles between renders means the approver did not see what
  was recorded (FU-17).
- **Never persist the XYFlow node/edge model.** The versioned specification is
  the source of truth; nodes and edges are a disposable projection. Persisting
  the render model would put `position` and `parentNode` inside an artefact
  that has to survive years and be read by an auditor (FU-18).

---

## Styling

- **Tailwind only.** No CSS modules, no styled-components, no inline `style`
  except for a computed value that cannot be a class.
- Never a raw colour in a component. `text-danger`, not `text-red-600`;
  `bg-card`, not `bg-white`. A hex outside `src/styles/tokens/` is a bug.
- Both themes are first-class. Every surface is legible in light and dark.
- A class run repeated three or more times becomes a component, not a
  constant holding a class string.
- **lucide-react** only for icons — 16px in dense UI, 20px in headers.

### The token layer

The design system ships as CSS custom properties and is the source of truth
for colour, type, spacing, elevation and motion.

```
src/styles/tokens/colors.css      palettes + semantic aliases, light and dark
src/styles/tokens/typography.css  the composed --type-* roles
src/styles/tokens/spacing.css     space ladder, density, layout widths
src/styles/tokens/elevation.css   layered shadows, scrim, blur
src/styles/tokens/motion.css      durations and easings
src/styles/tokens/shadcn.css      shadcn's variable contract, mapped onto the palette
src/styles/theme.css              the Tailwind bridge — the only @theme block
```

- **Never paste shadcn's stock values over `tokens/shadcn.css`.** That file is
  what makes a component from the registry adopt the brand with no override.
- A new colour utility is a line in `theme.css`, never a hex in a component.
- Values that flip with the theme go in `@theme inline` so the utility emits a
  `var()`; constants go in `@theme static`.
- Tailwind's own scale names carry the brand values: `text-sm` is 13px,
  `text-base` 15px, `rounded-xl` 16px, `shadow-sm` the layered brand shadow.
  Spacing uses Tailwind's numeric scale, which is the same 4px base.

### Colour has rules

- **Fuse Aqua is the only hue allowed to carry an action.** One primary action
  per view, plus the active nav state and agent identity. Nothing decorative.
- Semantic colour is strictly functional — run status, verdicts, limits.
- **Colour never carries meaning alone.** An agent state is a dot *and* a
  label; a verdict chip reads "Bloquear", not just red. Use `StateDot` with
  the word beside it, never the dot on its own.
- Machine-generated text is mono with `tabular-nums` — run ids, costs,
  latencies, policy codes, hashes. Human-authored text is sans. That split is
  the system's strongest signal; use the `Mono` component rather than
  restating the classes.

### Fonts

Geist and Geist Mono are **self-hosted** through `@fontsource-variable`. The
console runs inside the customer's network: a webfont CDN would both break in
an air-gapped install and report every page view to a third party. Never
reintroduce a Google Fonts import.

---

## Accessibility

- Every interactive element is reachable and operable by keyboard.
- Visible focus everywhere; never `outline: none` without a replacement.
- Every input has an associated `Label`. Placeholder is not a label.
- Icon-only buttons carry an accessible name.
- Respect `prefers-reduced-motion`.
- Colour never carries meaning alone — pair it with text or an icon. A verdict
  chip reads "Blocked", not just red.

---

## Required states

Every view that loads data implements four states, and a PR without them is
incomplete:

1. **Loading** — `Skeleton` shaped like the real content, never a bare spinner
2. **Empty** — explains what would appear here and the action that creates it
3. **Error** — what failed and what to do, with a retry. Never a stack trace
4. **Loaded**

---

## Tests

- **Vitest + Testing Library**, tests in `__tests__/` inside the feature.
- Query by role, label and visible text. `getByTestId` is a last resort.
- Test what the user does, not what the component stores. No snapshot tests
  without a stated reason; no "renders without crashing".
- `userEvent`, never `fireEvent`, unless the event has no user equivalent.
- Mock at the network boundary (MSW), never the component's own hooks.
- Maximum 25 lines per test.

---

## Definition of done

- [ ] No hand-written control — every one comes from `@/components/ui`
- [ ] No user-visible string outside i18n, both locales updated
- [ ] Loading, empty and error states implemented
- [ ] Keyboard reachable, visible focus, accessible names on icon buttons
- [ ] Legible in light and dark
- [ ] No component over 150 lines, no `any`
- [ ] Tests exercise real interactions
- [ ] `npm run lint` and `npm run test` clean
