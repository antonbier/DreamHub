<!--
  The one plain sentence a tab opens with, and — on tabs that have
  settings — the gear that opens them.
-->
<template>
  <div class="mt-3 flex items-start gap-3">
    <p class="min-w-0 flex-1 text-[13px] text-muted">{{ text }}</p>
    <button
      v-if="gearLabel"
      type="button"
      class="tap -my-1 shrink-0 rounded-sm p-1.5 text-muted hover:bg-tile-2 hover:text-text"
      :aria-label="gearLabel"
      :title="gearLabel"
      @click="emit('gear')"
    >
      <Icon name="settings" :size="18" />
    </button>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/Icon.vue'

defineProps<{ text: string; gearLabel?: string }>()
const emit = defineEmits<{ gear: [] }>()
</script>
