<!--
  Which box the page is about, above every page that is about a box — with
  two or more boxes only (a household with one sees nothing new). The top
  bar's selector says it too, but a page you scroll or screenshot should say
  it itself. Settings are DreamHub's, not a box's, and show no mark.
-->
<template>
  <div v-if="boxes.multi && boxes.active && !route.path.startsWith('/settings')" class="mx-auto -mb-3 max-w-content px-6 pt-4 below-md:px-4">
    <span class="inline-flex items-center gap-1.5 text-[12.5px] text-muted">
      <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: `var(--dh-profile-${boxes.active.color || 'slate'})` }" aria-hidden="true" />
      {{ t('boxes.pageMark', { box: boxes.active.name }) }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useBoxesStore } from '@/stores/boxes'

const { t } = useI18n()
const boxes = useBoxesStore()
const route = useRoute()
</script>
