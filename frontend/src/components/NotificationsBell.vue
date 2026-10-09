<!--
  The top bar's notification inbox (CONCEPT.md, "Notifications"). Two
  parts: *current* issues, derived live from data already on hand (hub
  problems from the navigation store — the same ones the sidebar's status
  dots explain — and a clock-skew check); and *recent activity*, the core
  event bus's real, persisted feed (docs/archive/V1_PLAN.md, WP7 —
  GET /api/v1/notifications, kept live over SSE by
  stores/notifications.ts): recording started/finished, disk above 90%.
  Opening the panel marks everything read.
-->
<template>
  <DropdownMenu>
    <template #trigger>
      <button
        type="button"
        :aria-label="t('topbar.notifications')"
        class="relative flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
        @click="onOpen"
      >
        <Icon name="bell" :size="18" />
        <span v-if="issues.length > 0 || notifications.unread > 0" class="absolute end-1 top-1 h-1.5 w-1.5 rounded-full bg-warn" />
      </button>
    </template>
    <template #panel>
      <div class="max-w-[320px] px-3 pb-2 pt-1.5">
        <div class="mb-1.5 text-[12.5px] text-faint">{{ t('notifications.title') }}</div>
        <p v-if="issues.length === 0" class="py-1 text-[13px] text-muted">{{ t('notifications.allOk') }}</p>
        <ul v-else class="flex flex-col">
          <li v-for="(issue, i) in issues" :key="i" class="flex items-start gap-2 border-b border-line-soft py-2 last:border-none">
            <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" :class="issue.level === 'warn' ? 'bg-warn' : 'bg-faint'" />
            <span class="min-w-0 flex-1 text-[13px] text-text">{{ issue.text }}</span>
          </li>
        </ul>

        <template v-if="notifications.items.length > 0">
          <div class="mb-1.5 mt-3 text-[12.5px] text-faint">{{ t('notifications.recent') }}</div>
          <ul class="flex flex-col">
            <li v-for="n in notifications.items" :key="n.id" class="border-b border-line-soft py-1.5 text-[12.5px] text-muted last:border-none">
              {{ describeNotification(n) }}
            </li>
          </ul>
        </template>
      </div>
    </template>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { useAuth } from '@/composables/useAuth'
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useNotificationsStore } from '@/stores/notifications'
import { useProblem } from '@/composables/useProblem'
import { useDeviceStatus, CLOCK_SKEW_WARN_MINUTES } from '@/composables/useDeviceStatus'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { Notification } from '@/api/types'

const { t, locale } = useI18n()
const { hubLabel } = useHubLabel()
const { timeZone } = useDisplayTimeZone()
const navigation = useNavigationStore()
const notifications = useNotificationsStore()
const { describe } = useProblem()
const { clockSkew, checkClock } = useDeviceStatus()

onMounted(() => {
  checkClock()
  if (!notifications.loaded) void notifications.load()
})

interface Issue {
  text: string
  level: 'warn' | 'muted'
}

// Setup problems (a hub's missing key, the box's clock) are the admin's to
// fix; a viewer wouldn't know what to do with them.
const { can } = useAuth()
const issues = computed<Issue[]>(() => {
  const out: Issue[] = []
  if (!can('settings.global')) return out
  if (clockSkew.value !== null && clockSkew.value >= CLOCK_SKEW_WARN_MINUTES) {
    out.push({ text: t('notifications.clockSkew', { minutes: clockSkew.value }), level: 'warn' })
  }
  for (const item of navigation.navigation?.sections.flatMap((s) => s.items) ?? []) {
    if (!item.status || item.status === 'available') continue
    for (const p of item.problems ?? []) {
      out.push({ text: `${hubLabel(item)}: ${describe(p)}`, level: item.status === 'degraded' ? 'warn' : 'muted' })
    }
  }
  return out
})

// notification.<key> mirrors problem.<code>'s nested-key convention
// (useProblem.ts) — key/params from the backend, never finished text.
// A "start"/"begin" Unix time in params is offered as {time}, in the
// display zone.
function describeNotification(n: Notification): string {
  const params: Record<string, unknown> = { ...(n.params as Record<string, unknown>) }
  const at = Number(params.start ?? params.begin)
  if (at > 0) {
    params.time = new Intl.DateTimeFormat(locale.value, { weekday: 'short', hour: '2-digit', minute: '2-digit', timeZone: timeZone.value }).format(at * 1000)
  }
  const translated = t(`notification.${n.key}`, params)
  return translated === `notification.${n.key}` ? n.key : translated
}

function onOpen() {
  void notifications.markRead()
}
</script>
