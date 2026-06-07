# TODO

## Goal

Replace the broken position-anchored feedback display with a simple vertical list that always shows all feedback, with optional inline anchor links that scroll the editor to the referenced heading.

## Tasks

### 1. Rewrite AlignedFeedbackPanel as a vertical list

- [x] Remove dependency on `positions` prop and `useFeedbackPositions` hook
- [x] Remove `useCollisionFreePositions` usage and absolute positioning logic
- [x] Render all feedback items in a simple vertical flex/stack layout (sorted by creation date)
- [x] For feedback with a `section_heading` anchor, render a clickable "§ Heading" link/badge that scrolls the editor to that heading
- [x] For feedback with only `line_number` anchor (legacy), show "L{n}" as informational text

→ Recorded as part of **i-3txo**

### 2. Add scroll-to-heading support

- [x] When anchor link is clicked, find the heading element in the editor DOM using existing `data-toc-id` attributes (match `section_heading` against `tocItems[].textContent`)
- [x] Use the same scroll pattern as `handleTocItemClick` — calculate position relative to scrollable container and `scrollTo({ behavior: 'smooth' })`
- [x] Pass an `onScrollToHeading(sectionHeading: string)` callback from SpecDetailPage to the feedback panel

→ Recorded as part of **i-3txo**

### 3. Update SpecDetailPage integration

- [x] Remove `useFeedbackPositions` hook usage from SpecDetailPage
- [x] Remove `positions` prop from AlignedFeedbackPanel invocation
- [x] Remove the complex absolute-positioned overlay layout (pointer-events-none wrapper)
- [x] Render the feedback panel as a normal sidebar element alongside the editor
- [x] Pass `onScrollToHeading` callback that looks up `data-toc-id` from `tocItems` and scrolls

→ Recorded as part of **i-3txo**

### 4. Update AddFeedbackDialog/FeedbackForm to always capture `section_heading`

- [x] In SpecDetailPage, resolve the nearest parent heading for the selected line from `tocItems` (find the last tocItem whose DOM position is at or above the selected line)
- [x] Pass `sectionHeading` to AddFeedbackDialog alongside `lineNumber`
- [x] In FeedbackForm, always include `section_heading` in the anchor (derived from nearest parent heading to the line number) — matching the CLI's `findContainingSection` behaviour
- [x] Show the section heading in the anchor info display (e.g. "§ Requirements" alongside "Line 42")
- [x] When no `lineNumber` is provided (general feedback), do not set a `section_heading` either

→ Recorded as **i-7a5h**

### 5. Update FeedbackAnchor badges in SpecViewer (source mode)

- [x] Keep existing inline count badges in source view
- [x] On badge click, scroll the feedback panel sidebar to show the corresponding feedback card (use `element.scrollIntoView`)

→ Recorded as **i-2ts8**

### 6. Clean up unused code

- [x] Remove `useFeedbackPositions.ts` hook (no longer needed)
- [x] Remove `useCollisionFreePositions.ts` hook if no other consumers exist
- [x] Remove `FeedbackMark.ts` Tiptap extension if unused elsewhere

→ Recorded as **i-8qpv**

### 7. Verify

- [ ] Test with spec that has anchored feedback — all feedback visible in panel
- [ ] Test with spec that has un-anchored (general) feedback — still displays
- [ ] Test anchor link click scrolls editor to correct heading
- [ ] Test source view badge click scrolls to feedback in panel
- [ ] Test adding new feedback captures the nearest section heading in the anchor

## Notes

- The current system tried to spatially align feedback cards next to their anchor points in the document, but since TiptapEditor (formatted mode) never emits `data-line-number` attributes, the position lookup always fails and anchored feedback is silently dropped.
- The new approach is more robust: always show everything, use anchors only as navigation hints.
- `section_heading` is the most reliable anchor field since headings are stable across edits; `line_number` drifts with any edit.
- The CLI already does this correctly: `createFeedbackAnchor` calls `findContainingSection` which walks backward from the line to find the nearest markdown heading. The frontend should mirror this by resolving the heading from `tocItems`.
- The existing ToC system already has heading discovery (`data-toc-id` attributes, `tocItems` state) — we reuse this for scroll-to-heading rather than building a parallel mechanism.
- `handleTocItemClick` in SpecDetailPage already implements the scroll-to-heading pattern we need.
- The server is a passthrough for anchor data — no server changes needed.

## Dependency Graph

```
i-3txo (core rewrite) ──blocks──→ i-7a5h (add feedback heading capture)
       │                          
       ├──blocks──→ i-2ts8 (source view scroll) ──blocks──→ i-8qpv (cleanup)
       │                                                        ↑
       └──blocks────────────────────────────────────────────────┘
```

All implement spec **s-7ld0**.
