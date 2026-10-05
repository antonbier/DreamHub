<!-- Shared chrome for every onboarding step: title, body slot, footer nav. -->
<template>
  <div>
    <h1 class="mb-1.5 text-[21px] font-bold">{{ title }}</h1>
    <p v-if="subtitle" class="mb-5 max-w-[440px] text-[13.5px] text-muted">{{ subtitle }}</p>
    <div v-else class="mb-5" />
    <div class="mb-6">
      <slot />
    </div>
    <p v-if="error" class="mb-4 text-[13px] text-warn">{{ error }}</p>
    <div class="flex items-center gap-3">
      <button
        v-if="showBack"
        type="button"
        class="rounded px-3.5 py-1.5 text-[13.5px] text-muted hover:bg-tile-2"
        @click="$emit('back')"
      >
        {{ t('onboarding.back') }}
      </button>
      <button
        type="button"
        class="rounded bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
        :disabled="nextDisabled || busy"
        @click="$emit('next')"
      >
        {{ busy ? t('onboarding.working') : nextLabel ?? t('onboarding.next') }}
      </button>
      <button
        v-if="showSkip"
        type="button"
        class="ms-1 text-[13px] text-muted hover:text-text"
        @click="$emit('skip')"
      >
        {{ t('onboarding.skip') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  title: string
  subtitle?: string
  showBack?: boolean
  showSkip?: boolean
  nextLabel?: string
  nextDisabled?: boolean
  busy?: boolean
  error?: string
}>()
defineEmits<{ next: []; back: []; skip: [] }>()
const { t } = useI18n()
</script>
