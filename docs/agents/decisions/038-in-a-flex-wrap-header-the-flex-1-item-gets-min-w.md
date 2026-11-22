# 38. In a `flex-wrap` header, the `flex-1` item gets `min-w-[240px]` — not `min-w-0`

**Location:** `frontend/src/routes/Downloads.svelte` (title block, at the top of the template). Covered by `frontend/tests/smoke/layout.spec.ts` ("subtitle is not squeezed to one word per line").

**What it looks like:** the title/subtitle block is `min-w-[240px] flex-1`, contradicting the `min-w-0` that decision 37 requires on grid/flex items in the rest of the app.

**Why it's right:** they are opposite problems. `min-w-0` is for an item that **contains a scroll strip** and therefore needs to be able to shrink below its content. Here the item is a neighbour of a wide box (the ↓/↑ bandwidth bar, 433px of max-content) inside a `flex-wrap` container, and a flex container's wrap decides by each item's **hypothetical main size** — which for `flex-1` (flex-basis 0%) is ~0, and `min-width` is exactly what enters that calculation. With `min-w-0` the title contributed ~0, the bar never went to the next line and the title was left with the leftovers: measured, 7 lines of subtitle at 500px and 4 at 768px, one word per line. The 240px floor pushes the bar to the next line at any width where it does not fit.

**Don't "fix" by:** swapping to `min-w-0` "to match the rest of the app" (that is the bug back — decision 37's rule is for an item that hosts scroll, not for an item neighbouring wide content); solving it with a breakpoint (`flex-col sm:flex-row`) — that fixes 375px and leaves the 500–768px band squeezed, which is exactly where the symptom was worst; assuming `flex-wrap` alone already wraps the line (it wraps by hypothetical size, not by the final size after flexing).
