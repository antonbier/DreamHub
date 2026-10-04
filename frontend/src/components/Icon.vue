<!--
  A small, self-drawn line-icon set (DESIGN.md: "no emoji as icons"; bundled,
  no external icon font/CDN — PRINCIPLES.md). Icon names match hub.Icon
  values from backend/internal/hub/v1.go plus a few UI-only names.
-->
<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.75"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    class="shrink-0"
  >
    <g v-html="markup" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })

const ICONS: Record<string, string> = {
  dashboard:
    '<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>',
  settings:
    '<circle cx="12" cy="12" r="3"/><path d="M12 3v2.4M12 18.6V21M4.2 4.2l1.7 1.7M18.1 18.1l1.7 1.7M3 12h2.4M18.6 12H21M4.2 19.8l1.7-1.7M18.1 5.9l1.7-1.7"/>',
  recordings:
    '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M9.8 9.3v5.4l4.6-2.7-4.6-2.7z" fill="currentColor" stroke="none"/>',
  series:
    '<rect x="3" y="4" width="14" height="4" rx="1"/><rect x="5.5" y="10" width="14" height="4" rx="1"/><rect x="8" y="16" width="14" height="4" rx="1"/>',
  guide:
    '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9.5h18M8 4v-1.2M16 4v-1.2"/><path d="M6.5 13h2M6.5 16.5h2M11 13h2M11 16.5h2M15.5 13h2M15.5 16.5h2"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  radio: '<rect x="3" y="8" width="18" height="12" rx="2"/><path d="M7 8l10-5"/><circle cx="15.5" cy="14" r="3"/><path d="M6.5 12h3M6.5 15h3M6.5 18h3"/>',
  timers: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l3 2M9.5 2h5"/>',
  recommendations:
    '<path d="M12 3l1.8 4.6L18 9l-4.2 1.4L12 15l-1.8-4.6L6 9l4.2-1.4L12 3z"/><path d="M18.3 15.2l.8 1.9 1.9.8-1.9.8-.8 1.9-.8-1.9-1.9-.8 1.9-.8z"/>',
  system:
    '<rect x="3" y="4" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><path d="M7 7h.01M7 17h.01"/>',
  admin:
    '<path d="M12 3l7 3v5c0 4.8-3 8.4-7 10-4-1.6-7-5.2-7-10V6l7-3z"/><path d="M9 12.2l2 2 4-4.2"/>',
  diagnostics:
    '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M6 13h2.7l1.3-3.5 2.4 7 1.3-3.5H18"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="M20 20l-4.8-4.8"/>',
  bell: '<path d="M6 10a6 6 0 0 1 12 0c0 4 1.5 5.5 1.5 5.5h-15S6 14 6 10z"/><path d="M9.5 18.5a2.5 2.5 0 0 0 5 0"/>',
  globe:
    '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.6 3.8 5.7 3.8 9s-1.3 6.4-3.8 9c-2.5-2.6-3.8-5.7-3.8-9S9.5 5.6 12 3z"/>',
  'chevron-left': '<path d="M15 5l-7 7 7 7"/>',
  'chevron-right': '<path d="M9 5l7 7-7 7"/>',
  'chevron-up': '<path d="M5 15l7-7 7 7"/>',
  'chevron-down': '<path d="M5 9l7 7 7-7"/>',
  power: '<path d="M12 3v8"/><path d="M6.6 6.6a7.5 7.5 0 1 0 10.8 0"/>',
  'volume-up': '<path d="M4 9.5h3.5L12 5.5v13l-4.5-4H4z"/><path d="M15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11"/>',
  'volume-down': '<path d="M4 9.5h3.5L12 5.5v13l-4.5-4H4z"/><path d="M15.5 9a4 4 0 0 1 0 6"/>',
  mute: '<path d="M4 9.5h3.5L12 5.5v13l-4.5-4H4z"/><path d="M16 9.5l5 5M21 9.5l-5 5"/>',
  play: '<path d="M7 5l12 7-12 7z"/>',
  pause: '<path d="M8 5v14M16 5v14"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="1"/>',
  rewind: '<path d="M11 6l-7 6 7 6zM20 6l-7 6 7 6z"/>',
  'fast-forward': '<path d="M13 6l7 6-7 6zM4 6l7 6-7 6z"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5.5M12 7.5h.01"/>',
  back: '<path d="M9 7L4.5 11.5 9 16"/><path d="M4.5 11.5H15a4.5 4.5 0 0 1 0 9h-2"/>',
  refresh: '<path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3"/><path d="M19.5 4.5v4.2h-4.2"/>',
  'panel-end': '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/>',
  sun: '<circle cx="12" cy="12" r="4.3"/><path d="M12 2.5v2.3M12 19.2v2.3M4.3 4.3l1.7 1.7M18 18l1.7 1.7M2.5 12h2.3M19.2 12h2.3M4.3 19.7L6 18M18 6l1.7-1.7"/>',
  moon: '<path d="M20 14.5A8.5 8.5 0 1 1 9.5 4a7 7 0 0 0 10.5 10.5z"/>',
  laptop: '<rect x="3.5" y="4.5" width="17" height="11" rx="1.5"/><path d="M2 19.5h20"/>',
  user: '<circle cx="12" cy="8.5" r="3.5"/><path d="M5 20c1-3.8 4-6 7-6s6 2.2 7 6"/>',
  warning: '<path d="M12 4l9 16H3l9-16z"/><path d="M12 10v4M12 17h.01"/>',
  close: '<path d="M6 6l12 12M18 6L6 18"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  control:
    '<rect x="7" y="2.5" width="10" height="19" rx="3"/><circle cx="12" cy="7.2" r="1.4"/><path d="M9.3 11.5h5.4M9.3 14.5h5.4M9.3 17.5h5.4"/>',
}

const markup = computed(() => ICONS[props.name] ?? ICONS.dashboard)
</script>
