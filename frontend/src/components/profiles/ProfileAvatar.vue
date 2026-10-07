<!--
  A profile's initial on its color (docs/PROFILES_PLAN.md: name and color,
  no photos). Decorative next to the name; the name itself is always shown
  as text too.
-->
<template>
  <span
    class="inline-flex shrink-0 select-none items-center justify-center rounded-full font-bold text-bg"
    :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${Math.round(size * 0.45)}px`, background: `var(--dh-profile-${color || 'blue'})` }"
    aria-hidden="true"
  >
    {{ initial }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ name: string; color?: string; size?: number }>(), { color: 'blue', size: 40 })
const initial = computed(() => Array.from(props.name.trim())[0]?.toLocaleUpperCase() ?? '?')
</script>
