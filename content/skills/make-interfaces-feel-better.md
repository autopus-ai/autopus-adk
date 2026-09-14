---
name: make-interfaces-feel-better
description: >
  Use when building or reviewing UI polish: typography, hit areas, nested
  radius, optical alignment, shadows, media outlines, micro-interactions, and
  animation performance.
triggers:
  - UI polish
  - interface polish
  - make it feel better
  - visual details
  - micro-interactions
  - typography
  - border radius
  - optical alignment
  - shadows
  - animation performance
  - hover states
  - 프론트엔드 디테일
  - UI 디테일
category: methodology
bundles:
  - frontend
level1_metadata: "UI detail pass: typography wrapping, tabular numbers, concentric radius, optical alignment, shadows, image outlines, hit areas, explicit transitions, restrained micro-interactions"
---

# Make Interfaces Feel Better

Use this skill after the core layout and behavior exist, or during review when a UI feels technically correct but visually rough. It focuses on small design-engineering details that compound into a more polished interface.

This is a polish pass, not a new visual identity. Read the project's `DESIGN.md`, tokens, shared components, and existing screens first. Apply the details through the project's current stack and naming conventions.

## Detail Pass

### Typography

- Use balanced wrapping for short headings and titles when the platform supports it.
- Use prettier wrapping for short-to-medium paragraphs, descriptions, captions, and list text.
- Leave long prose, code, and preformatted text on normal browser wrapping.
- Apply font smoothing once at the root when the stack supports it, rather than per element.
- Use tabular numerals for changing numbers: timers, counters, prices, dashboard metrics, scoreboards, and numeric table columns.

### Surfaces

- For nested rounded surfaces, make the outer radius visually account for the padding between the outer and inner element.
- Use optical alignment when geometric centering looks wrong, especially text+icon buttons, play icons, arrows, carets, stars, and asymmetric SVGs.
- Use shadows or ring-like shadow layers for element depth when a solid border would look harsh on varied backgrounds.
- Keep real dividers, table boundaries, and accessibility-critical input outlines as borders when they are serving structure.
- Add subtle inset outlines to images/media when the surrounding system uses depth or framed surfaces; use neutral black or white opacity, not tinted brand colors.
- Interactive controls should expose a stable hit area: at least 44px where touch is possible, and never below 40px for compact pointer-only controls unless the surrounding component already expands the target.
- Do not let invisible hit areas overlap.

### Motion

- Use transitions for interactive state changes so they can be interrupted and retargeted mid-action.
- Reserve keyframe or staged motion for one-shot entrance, loading, or storytelling sequences.
- Split entrance motion into semantic chunks, then stagger lightly so the page reads in order.
- Make exits shorter and quieter than entrances; use small fixed movement instead of large dramatic travel unless spatial context truly matters.
- Animate contextual icons with opacity, scale, and light blur rather than abruptly toggling visibility.
- Use a restrained press scale on tactile controls; avoid exaggerated shrink effects.
- Respect reduced-motion settings and keep critical state changes understandable with motion disabled.

### Performance

- Do not use `transition: all` or broad utility shorthands that animate every property.
- Specify only the properties that actually change, such as `opacity`, `transform`, `scale`, `filter`, `background-color`, or `box-shadow`.
- Add `will-change` only after observing first-frame stutter, and only for compositor-friendly properties such as transform, opacity, filter, or clip-path.
- Remove stale `will-change` once the interaction is no longer active or the performance issue is gone.

## Implementation Contract

- Prefer existing tokens, CSS variables, utility classes, and component primitives over raw values.
- Do not add a motion dependency only for this skill. Use the existing animation library if present; otherwise use CSS transitions.
- Keep component dimensions stable so hover, active, loading, icon-swap, and numeric updates do not shift layout.
- If the app already has a design system rule that conflicts with a polish heuristic, follow the local rule and record the tradeoff.
- For dense product surfaces, keep polish functional: improve scanability, target acquisition, state clarity, and perceived responsiveness.
- For marketing or visual-first surfaces, make the polish visible in the first viewport without adding decorative clutter.

## Review Output

When reviewing or reporting polish changes, group results by principle and use a compact table:

| Before | After |
| --- | --- |
| File and property that felt rough | File and property after the polish pass |

Omit empty principles. Each row should name the file and the specific property or component behavior that changed.

## Checklist

- [ ] Headings and short text wrap cleanly without orphan words.
- [ ] Dynamic numbers use tabular numerals where layout shift matters.
- [ ] Nested rounded surfaces are visually concentric.
- [ ] Icons and asymmetric shapes are optically aligned.
- [ ] Depth uses appropriate shadows, rings, or outlines without noisy borders.
- [ ] Images/media have subtle neutral outlines when they need separation.
- [ ] Interactive hit areas are large enough and do not overlap.
- [ ] Hover, focus, pressed, disabled, loading, enter, and exit states feel intentional.
- [ ] Interactive motion is interruptible.
- [ ] Reduced-motion users still get clear state changes.
- [ ] Transitions list exact properties; no `transition: all`.
- [ ] `will-change` is specific, temporary, and justified.

## Provenance

Adapted for Autopus from the MIT-licensed public skill `jakubkrehel/make-interfaces-feel-better` at commit `384562064fcdd99778fcbafd8729626fe6aab02f`. The upstream material is design-engineering evidence and inspiration; this canonical Autopus skill is self-contained and should not fetch or install the upstream repository during harness execution.
