<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <h1 class="text-[26px] font-bold">{{ t('notFound.title') }}</h1>
    <p class="mt-2 text-[14px] text-muted">{{ t('notFound.body') }}</p>
    <RouterLink to="/" class="mt-4 inline-block text-[13.5px] text-accent hover:underline">
      {{ t('notFound.backToDashboard') }}
    </RouterLink>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
