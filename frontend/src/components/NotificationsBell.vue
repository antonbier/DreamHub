<!--
  The top bar's notification inbox (CONCEPT.md, "Notifications"). Two
  parts: *current* issues, derived live from data already on hand (hub
  problems from the navigation store — the same ones the sidebar's status
  dots explain — and a clock-skew check); and *recent activity*, the real
  hub.statusChanged entries the backend now writes to the change log
  (internal/hub/watcher.go) — a genuine, if partial, history. A full
  event-bus-backed inbox covering every hub action is still open
  (ROADMAP.md).
-->
<template>
  <DropdownMenu>
    <template #trigger>
      <button
        type="button"
        :aria-label="t('topbar.notifications')"
        class="relative flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
        @click="loadRecent"
      >
        <Icon name="bell" :size="18" />
        <span v-if="issues.length > 0" class="absolute end-1 top-1 h-1.5 w-1.5 rounded-full bg-warn" />
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

        <template v-if="recent.length > 0">
          <div class="mb-1.5 mt-3 text-[12.5px] text-faint">{{ t('notifications.recent') }}</div>
          <ul class="flex flex-col">
            <li v-for="entry in recent" :key="entry.id" class="border-b border-line-soft py-1.5 text-[12.5px] text-muted last:border-none">
              {{ entry.summary }}
            </li>
          </ul>
        </template>
      </div>
    </template>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import { api } from '@/api/client'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'
import { useDeviceStatus, CLOCK_SKEW_WARN_MINUTES } from '@/composables/useDeviceStatus'
import type { ChangeEntry } from '@/api/types'

const { t } = useI18n()
const navigation = useNavigationStore()
const { describe } = useProblem()
const { clockSkew, checkClock } = useDeviceStatus()

onMounted(checkClock)

interface Issue {
  text: string
  level: 'warn' | 'muted'
}

const issues = computed<Issue[]>(() => {
  const out: Issue[] = []
  if (clockSkew.value !== null && clockSkew.value >= CLOCK_SKEW_WARN_MINUTES) {
    out.push({ text: t('notifications.clockSkew', { minutes: clockSkew.value }), level: 'warn' })
  }
  for (const item of navigation.navigation?.sections.flatMap((s) => s.items) ?? []) {
    if (!item.status || item.status === 'available') continue
    for (const p of item.problems ?? []) {
      out.push({ text: `${t(item.label)}: ${describe(p)}`, level: item.status === 'degraded' ? 'warn' : 'muted' })
    }
  }
  return out
})

const recent = ref<ChangeEntry[]>([])
async function loadRecent() {
  try {
    const { changes } = await api.changes(50)
    recent.value = changes.filter((c) => c.action === 'hub.statusChanged').slice(0, 5)
  } catch {
    recent.value = []
  }
}
</script>
