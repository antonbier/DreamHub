<!--
  The small settings dialog behind a tab's gear (Packages: feeds, Docker:
  Docker settings). A plain modal like the other dialogs (DESIGN.md): the
  title, a Close text button, the content; Escape, Close and the backdrop
  close it and focus goes back to the gear.
-->
<template>
  <div class="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-black/60 px-4 py-[6vh] below-sm:px-0 below-sm:py-0" @click.self="emit('close')">
    <div
      ref="dialogEl"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      class="w-full max-w-[560px] rounded-lg border border-line bg-tile p-5 shadow-lg below-sm:min-h-full below-sm:rounded-none below-sm:border-0"
    >
      <div class="flex items-center gap-3">
        <h2 :id="titleId" class="text-[17px] font-bold text-text">{{ title }}</h2>
        <button ref="closeButton" type="button" class="ms-auto rounded-sm px-2.5 py-1 text-[13px] text-text hover:bg-tile-2" @click="emit('close')">
          {{ t('plan.close') }}
        </button>
      </div>
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'

defineProps<{ title: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const titleId = useId()
const closeButton = ref<HTMLButtonElement | null>(null)
const opener = document.activeElement as HTMLElement | null

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  document.addEventListener('keydown', onKey)
  closeButton.value?.focus()
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  opener?.focus?.()
})
</script>
