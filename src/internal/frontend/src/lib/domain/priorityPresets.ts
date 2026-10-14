import type { Priorities } from "../api/client.js"
import * as m from "../i18n/messages.js"

/**
 * A preset is a one-time stamp, not a stored mode: it applies, becomes an ordinary editable list,
 * and the existing "Save" button persists it. Nothing new in config.json.
 *
 * It reorders instead of stamping a literal array — it only promotes tokens the user already has.
 * That way a hand-added token moves down instead of disappearing, and the canonical tokens keep
 * living in one place (reCodecPatterns, in the backend).
 *
 * `label`/`desc` are message functions, called inside the screen's reactive `$locale` block.
 */
export type ListPreset = { key: string; label: () => string; desc: () => string; first: string[] }

export const PRESETS: Partial<Record<keyof Priorities, ListPreset[]>> = {
  codecs: [
    {
      key: "compat",
      label: m.priorities_preset_compat_label,
      desc: m.priorities_preset_compat_desc,
      first: ["h.264"],
    },
    {
      key: "space",
      label: m.priorities_preset_space_label,
      desc: m.priorities_preset_space_desc,
      first: ["av1", "hevc"],
    },
  ],
}

/** Returns `list` with the items of `first` it contains promoted to the top, in `first`'s order. */
export function applyPreset(list: string[], first: string[]): string[] {
  const promote = first.filter((v) => list.includes(v))
  return [...promote, ...list.filter((v) => !promote.includes(v))]
}
