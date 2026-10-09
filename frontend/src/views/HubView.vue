<!--
  Placeholder for every hub route until that hub gets its own screens
  (ROADMAP.md, "v1 — hubs"). Looks the matching entry up in the navigation
  store so the title, icon and — importantly — the status and problem
  explanation (PRINCIPLES.md, "Hub status: always explain what's missing")
  are real even though the hub's content isn't built yet.
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div v-if="item" class="flex items-start gap-3">
      <Icon :name="item.icon" :size="24" class="mt-0.5 text-muted" />
      <div>
        <h1 class="text-[26px] font-bold">{{ t('hubPlaceholder.title', { name: hubLabel(item) }) }}</h1>
        <p class="mt-2 max-w-[640px] text-[14px] text-muted">{{ t('hubPlaceholder.body') }}</p>
        <div v-if="item.status && item.status !== 'available'" class="mt-4 rounded border border-line bg-tile p-4">
          <div class="mb-1.5 flex items-center gap-2 text-[13.5px] font-semibold">
            <span
              class="h-1.5 w-1.5 rounded-full"
              :class="item.status === 'degraded' ? 'bg-warn' : 'bg-faint'"
            />
            {{ t(`status.${item.status}`) }}
          </div>
          <ul class="flex flex-col gap-1 text-[13px] text-muted">
            <li v-for="(p, i) in item.problems ?? []" :key="i">
              {{ describe(p) }}
              <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
            </li>
          </ul>
        </div>
      </div>
    </div>
    <p v-else class="text-[14px] text-muted">{{ t('common.loading') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useHubLabel } from '@/composables/useHubLabel'
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/Icon.vue'
import { useNavigationStore } from '@/stores/navigation'
import { useProblem } from '@/composables/useProblem'

const route = useRoute()
const { t } = useI18n()
const { hubLabel } = useHubLabel()
const navigation = useNavigationStore()
const { describe, describeFix } = useProblem()

const item = computed(() =>
  navigation.navigation?.sections.flatMap((s) => s.items).find((i) => i.path === route.path),
)
</script>
