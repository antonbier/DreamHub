<!--
  A small popover menu: trigger slot + panel slot. Closes on an outside
  click or Escape (DESIGN.md, Interaction: "Esc closes dialogs, menus and
  the drawer"). Used by the language switcher and the user menu.
-->
<template>
  <div ref="root" class="relative">
    <span @click="toggle">
      <slot name="trigger" :open="open" />
    </span>
    <div
      v-if="open"
      class="absolute inset-e-0 z-20 mt-2 min-w-[220px] rounded-lg border border-line bg-tile py-1.5 shadow-lg"
      @keydown.esc="close"
    >
      <slot name="panel" :close="close" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const open = ref(false)
const root = ref<HTMLElement | null>(null)

function toggle() {
  open.value = !open.value
}
function close() {
  open.value = false
}
function onDocClick(e: MouseEvent) {
  if (open.value && root.value && !root.value.contains(e.target as Node)) close()
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') close()
}

onMounted(() => {
  document.addEventListener('mousedown', onDocClick)
  document.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocClick)
  document.removeEventListener('keydown', onKeydown)
})

defineExpose({ close })
</script>
