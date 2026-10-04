<!--
  Under General (SettingsView.vue): DreamHub as an app on the phone or
  desktop (src/lib/pwa.ts). What it says depends on where it's opened:
  already installed, an Install button when the browser offers one, the
  iOS share-sheet steps, or why the browser won't offer it (plain http).
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <div class="flex items-start gap-4">
      <img src="/icons/icon-192.png" alt="" class="h-12 w-12 shrink-0 rounded-lg" />
      <div class="min-w-0 flex-1">
        <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('settings.install.title') }}</h2>
        <p class="text-[13px] text-muted">{{ t(`settings.install.${kind}`) }}</p>
        <button
          v-if="kind === 'prompt'"
          type="button"
          class="mt-3 rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg"
          @click="pwa.install()"
        >
          {{ t('settings.install.button') }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePwaInstall } from '@/lib/pwa'

const { t } = useI18n()
const pwa = usePwaInstall()
// Re-evaluated when the deferred prompt arrives or the app gets installed.
const kind = computed(() => {
  void pwa.deferred.value
  void pwa.installed.value
  return pwa.platform()
})
</script>
