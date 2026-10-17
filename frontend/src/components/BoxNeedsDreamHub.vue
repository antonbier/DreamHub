<!--
  One sentence for a page (or part of one) that works only on a box with
  DreamHub installed (a Full box), shown when the box on screen is a Basic
  one, with the way to Settings → Boxes. A deep link such as /b/<id>/apps
  lands here instead of on an error.
-->
<template>
  <p class="text-[13.5px] text-muted">
    {{ t(part ? 'boxes.needsDreamHubPart' : 'boxes.needsDreamHub', { box: name }) }}
    <RouterLink to="/settings/boxes" class="text-accent hover:underline">{{ t('boxes.needsDreamHubLink') }}</RouterLink>
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useBoxesStore } from '@/stores/boxes'

// part: only a part of the page needs it (Device: change log, administration).
defineProps<{ part?: boolean }>()
const { t } = useI18n()
const boxes = useBoxesStore()
const name = computed(() => boxes.active?.name ?? '')
</script>
