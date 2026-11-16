// The webhook events the backend fires (notifications.eventString), grouped the way the form shows
// them. Keys are the backend's and are what a WebhookPreset.events stores.
import * as m from '../i18n/messages.js'

export type EventGroup = 'episodes' | 'library' | 'system'

export const EVENT_GROUPS: { group: EventGroup; events: string[] }[] = [
  { group: 'episodes', events: ['new_episode', 'download_failed', 'download_completed', 'episode_deleted'] },
  { group: 'library', events: ['episode_limit_reached', 'episode_stuck', 'season_complete'] },
  { group: 'system', events: ['library_missing', 'daemon_crashed', 'check_failing', 'update_available', 'disk_low'] },
]

export const ALL_EVENTS: string[] = EVENT_GROUPS.flatMap(g => g.events)

/**
 * The events a new message preset (ntfy, Discord, Telegram…) starts with: every episode event, the
 * system alerts, and episode_stuck — it is where "no torrent found" is reported once it lasts. The
 * other library events are opt-in: they are reminders about the user's own habits, not problems. Existing webhooks are never touched: an upgrade turns
 * nothing new on for anyone.
 */
export const MESSAGE_PRESET_EVENTS: string[] = [
  ...EVENT_GROUPS[0].events,
  'library_missing',
  'daemon_crashed',
  'check_failing',
  'update_available',
  'disk_low',
  'episode_stuck',
]

export function eventLabel(event: string): string {
  switch (event) {
    case 'new_episode':
      return m.notifications_event_new_episode()
    case 'download_failed':
      return m.notifications_event_download_failed()
    case 'download_completed':
      return m.notifications_event_download_completed()
    case 'episode_deleted':
      return m.notifications_event_episode_deleted()
    case 'library_missing':
      return m.notifications_event_library_missing()
    case 'daemon_crashed':
      return m.notifications_event_daemon_crashed()
    case 'check_failing':
      return m.notifications_event_check_failing()
    case 'update_available':
      return m.notifications_event_update_available()
    case 'disk_low':
      return m.notifications_event_disk_low()
    case 'episode_limit_reached':
      return m.notifications_event_episode_limit_reached()
    case 'episode_stuck':
      return m.notifications_event_episode_stuck()
    case 'season_complete':
      return m.notifications_event_season_complete()
    default:
      return event
  }
}

export function groupLabel(group: EventGroup): string {
  switch (group) {
    case 'episodes':
      return m.notifications_group_episodes()
    case 'library':
      return m.notifications_group_library()
    case 'system':
      return m.notifications_group_system()
  }
}
