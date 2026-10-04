<!--
  A row of day chips for GuideHub's day-based views: "Today", "Tomorrow",
  then weekday + date, in the display time zone. v-model is the day
  offset from today.
-->
<template>
  <div class="flex flex-wrap items-center gap-1 text-[13px]" role="radiogroup" :aria-label="t('guide.day')">
    <button
      v-for="d in days"
      :key="d.offset"
      type="button"
      role="radio"
      :aria-checked="modelValue === d.offset"
      class="rounded px-2.5 py-1"
      :class="modelValue === d.offset ? 'bg-tile-2 text-text ring-1 ring-line' : 'text-muted hover:bg-tile hover:text-text'"
      @click="$emit('update:modelValue', d.offset)"
    >
      {{ d.label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import { zonedUnix } from '@/lib/zonedTime'

const props = withDefaults(defineProps<{ modelValue: number; count?: number }>(), { count: 7 })
defineEmits<{ 'update:modelValue': [offset: number] }>()
const { t, locale } = useI18n()
const { timeZone } = useDisplayTimeZone()

const days = computed(() =>
  Array.from({ length: props.count }, (_, offset) => {
    if (offset === 0) return { offset, label: t('discovery.today') }
    if (offset === 1) return { offset, label: t('discovery.tomorrow') }
    const at = zonedUnix(timeZone.value, offset, 12, 0) * 1000
    return { offset, label: new Intl.DateTimeFormat(locale.value, { weekday: 'short', day: 'numeric', month: 'numeric', timeZone: timeZone.value }).format(at) }
  }),
)
</script>
