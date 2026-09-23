<!--
  The top bar's notification inbox (CONCEPT.md, "Notifications"). v1 has no
  event bus or hub-emitted events yet (ROADMAP.md) — rather than fabricate
  a history, this shows the *current* issues derived from data already on
  hand: hub problems from the navigation store (the same ones the sidebar's
  status dots explain) and a clock-skew check against GET /api/v1/health.
  A persisted, historical inbox is a separate, still-open item.
-->
<template>
  <DropdownMenu>
    <template #trigger>
      <button
        type="button"
        :aria-label="t('topbar.notifications')"
        class="relative flex h-8 w-8 items-center justify-center rounded text-muted hover:bg-tile-2 hover:text-text"
      >
        <Icon name="bell" :size="18" />
        <span
          v-if="issues.length > 0"
          class="absolute end-1 top-1 h-1.5 w-1.5 rounded-full bg-warn"
        />
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
      </div>
    </template>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import DropdownMenu from './DropdownMenu.vue'
import { clockSkewMinutes as fetchClockSkew } from '@/lib/clock'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'

const CLOCK_SKEW_WARN_MINUTES = 1

const { t } = useI18n()
const navigation = useNavigationStore()
const { describe } = useProblem()

const clockSkewMinutes = ref<number | null>(null)

onMounted(async () => {
  // A null result (backend unreachable) is already shown by App.vue's
  // banner; nothing extra to say here.
  clockSkewMinutes.value = await fetchClockSkew()
})

interface Issue {
  text: string
  level: 'warn' | 'muted'
}

const issues = computed<Issue[]>(() => {
  const out: Issue[] = []
  if (clockSkewMinutes.value !== null && clockSkewMinutes.value >= CLOCK_SKEW_WARN_MINUTES) {
    out.push({ text: t('notifications.clockSkew', { minutes: clockSkewMinutes.value }), level: 'warn' })
  }
  for (const item of navigation.navigation?.sections.flatMap((s) => s.items) ?? []) {
    if (!item.status || item.status === 'available') continue
    for (const p of item.problems ?? []) {
      out.push({ text: `${t(item.label)}: ${describe(p)}`, level: item.status === 'degraded' ? 'warn' : 'muted' })
    }
  }
  return out
})
</script>
