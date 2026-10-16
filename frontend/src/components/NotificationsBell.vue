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
        class="relative flex h-8 w-8 items-center justify-center rounded-sm text-muted hover:bg-tile-2 hover:text-text"
        @click="onOpen"
      >
        <Icon name="bell" :size="18" />
        <span v-if="issues.length > 0 || notifications.hasUnread" class="absolute inset-e-1 top-1 h-1.5 w-1.5 rounded-full bg-warn" />
      </button>
    </template>
    <template #panel="{ close }">
      <div class="max-w-[320px] px-3 pb-2 pt-1.5">
        <div class="mb-1.5 text-[12.5px] text-faint">{{ t('notifications.title') }}</div>
        <p v-if="issues.length === 0" class="py-1 text-[13px] text-muted">{{ t('notifications.allOk') }}</p>
        <ul v-else class="flex flex-col">
          <li v-for="(issue, i) in issues" :key="i" class="flex items-start gap-2 border-b border-line-soft py-2 last:border-none">
            <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full" :class="issue.level === 'warn' ? 'bg-warn' : 'bg-faint'" />
            <RouterLink v-if="issue.to" :to="issue.to" class="min-w-0 flex-1 text-[13px] text-accent hover:underline" @click="close()">{{ issue.text }}</RouterLink>
            <span v-else class="min-w-0 flex-1 text-[13px] text-text">{{ issue.text }}</span>
          </li>
        </ul>

        <!-- Browser notifications: only where the browser allows them. -->
        <div v-if="browserNotes.supported && (browserNotes.permission.value !== 'denied' || tried)" class="mt-3 border-t border-line-soft pt-2 text-[12.5px]">
          <button v-if="!browserNotes.active.value" type="button" class="text-accent hover:underline" @click="tried = true; browserNotes.enable()">
            {{ t('notifications.browserOn') }}
          </button>
          <template v-else>
            <span class="text-muted">{{ t('notifications.browserIsOn') }}</span>
            <button type="button" class="ms-2 text-accent hover:underline" @click="browserNotes.disable()">{{ t('notifications.browserOff') }}</button>
          </template>
          <p v-if="tried && browserNotes.permission.value === 'denied'" class="mt-1 text-faint">{{ t('notifications.browserDenied') }}</p>
        </div>

        <template v-if="notifications.items.length > 0">
          <div class="mb-1.5 mt-3 flex items-baseline justify-between gap-2 text-[12.5px] text-faint">
            <span>{{ t('notifications.recent') }}</span>
            <label v-if="boxes.multi" class="flex cursor-pointer items-center gap-1.5">
              <input type="checkbox" :checked="notifications.onlyThisBox" @change="notifications.setOnlyThisBox(($event.target as HTMLInputElement).checked)" />
              {{ t('notifications.onlyThisBox') }}
            </label>
          </div>
          <ul class="flex flex-col">
            <li v-for="n in notifications.visible" :key="n.id" class="border-b border-line-soft py-1.5 text-[12.5px] text-muted last:border-none">
              <RouterLink v-if="notificationLink(n)" :to="notificationLink(n)!" class="text-accent hover:underline" @click="close()">{{ describeNotification(n) }}</RouterLink>
              <template v-else>{{ describeNotification(n) }}</template>
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
import { useBrowserNotifications } from '@/composables/useBrowserNotifications'
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import type { RouteLocationRaw } from 'vue-router'
import { useUpdates } from '@/composables/useUpdates'
import { useNavigationStore } from '@/stores/navigation'
import { useNotificationsStore } from '@/stores/notifications'
import { useBoxesStore } from '@/stores/boxes'
import { useProblem } from '@/composables/useProblem'
import { useDeviceStatus, CLOCK_SKEW_WARN_MINUTES } from '@/composables/useDeviceStatus'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { Notification } from '@/api/types'

const { t, locale } = useI18n()
// The browser's "blocked" note only matters once someone asked for notifications.
const tried = ref(false)
const { hubLabel } = useHubLabel()
const { timeZone } = useDisplayTimeZone()
const navigation = useNavigationStore()
const notifications = useNotificationsStore()
const boxes = useBoxesStore()
const { describe } = useProblem()
const { clockSkew, checkClock } = useDeviceStatus()
const { summary: updates, reload: reloadUpdates } = useUpdates()

const browserNotes = useBrowserNotifications()
// A new item arrives while the page is in the background: say so there too.
watch(
  () => notifications.items[0]?.id,
  (id, before) => {
    const n = notifications.items[0]
    if (id !== undefined && before !== undefined && id !== before && !n.read && notifications.visible.includes(n)) browserNotes.show('DreamHub', describeNotification(n))
  },
)

onMounted(() => {
  checkClock()
  if (!notifications.loaded) void notifications.load()
})

interface Issue {
  text: string
  level: 'warn' | 'muted'
  // Where the fix is, when there is a page for it.
  to?: string
}

// Setup problems (a hub's missing key, the box's clock) are the admin's to
// fix; a viewer wouldn't know what to do with them.
const { can } = useAuth()
const issues = computed<Issue[]>(() => {
  const out: Issue[] = []
  // The same update count as the Apps page's header (one update notion).
  if (can('system') && updates.value && updates.value.count > 0) {
    out.push({ text: t('updates.title', { count: updates.value.count }, updates.value.count), level: 'muted', to: '/apps' })
  }
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
  const text = translated === `notification.${n.key}` ? n.key : translated
  // With several boxes the text says which one it is about.
  return n.box ? `${text} · ${n.box.name}` : text
}

// An entry that has a page to go to: the crash notice opens the app's logs,
// the update ones the Apps page.
function notificationLink(n: Notification): RouteLocationRaw | null {
  const params = n.params as Record<string, unknown>
  switch (n.key) {
    case 'app.crashLoop':
      return typeof params.appId === 'string' && params.appId ? { path: '/apps', query: { tab: 'docker', logs: params.appId } } : null
    case 'updates.applied':
    case 'updates.failed':
    case 'updates.rolledBack':
    case 'app.upgradable':
    case 'package.upgradable':
      return '/apps'
    default:
      return null
  }
}

function onOpen() {
  void notifications.markRead()
  if (can('system')) void reloadUpdates()
}
</script>
