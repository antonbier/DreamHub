<template>
  <OnboardingStepFrame :title="t('onboarding.language.title')" :subtitle="t('onboarding.language.subtitle')" @next="$emit('next')">
    <div class="flex flex-col gap-2">
      <button
        v-for="locale in SUPPORTED_LOCALES"
        :key="locale"
        type="button"
        class="flex items-center justify-between rounded border px-3.5 py-2.5 text-start text-[14px]"
        :class="preferences.language === locale ? 'border-accent bg-tile-2 text-text' : 'border-line text-muted hover:bg-tile-2'"
        @click="preferences.setLanguage(locale)"
      >
        {{ LOCALE_NAMES[locale] }}
        <span v-if="preferences.language === locale" class="text-accent">{{ '✓' }}</span>
      </button>
    </div>
  </OnboardingStepFrame>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import OnboardingStepFrame from './OnboardingStepFrame.vue'
import { usePreferencesStore } from '@/stores/preferences'
import { LOCALE_NAMES, SUPPORTED_LOCALES } from '@/i18n'

defineEmits<{ next: [] }>()
const { t } = useI18n()
const preferences = usePreferencesStore()
</script>
