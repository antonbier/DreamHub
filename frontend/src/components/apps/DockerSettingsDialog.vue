<!--
  "Docker settings" behind the Docker tab's gear: Docker's status, Set up
  Docker (App7) and Uninstall Docker (App10). Later packages add the
  storage location, auto-start and the like here (docs/APPSHUB_PLAN.md,
  App13). Both changes are plans with a preview.
-->
<template>
  <SettingsDialog :title="t('apps.docker.settingsTitle')" @close="emit('close')">
    <p v-if="!state" class="mt-4 text-[13px] text-faint">{{ unavailable ? t('apps.docker.unavailable') : t('common.loading') }}</p>
    <template v-else>
      <p class="mt-3 flex items-center gap-1.5 text-[13.5px]" :class="state.ready ? 'text-ok' : 'text-muted'">
        <span class="h-1.5 w-1.5 rounded-full" :class="state.ready ? 'bg-ok' : 'bg-faint'" />
        {{ statusText }}
      </p>
      <DockerSetupSection v-if="!state.ready || setupTouched" :state="state" :version="version" :unavailable="unavailable" @changed="changed('setup')" />
      <DockerUninstallSection v-if="installed || uninstallTouched" @changed="changed('uninstall')" />
    </template>
  </SettingsDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import DockerSetupSection from '@/components/apps/DockerSetupSection.vue'
import DockerUninstallSection from '@/components/apps/DockerUninstallSection.vue'
import SettingsDialog from '@/components/apps/SettingsDialog.vue'
import type { DockerSetupState } from '@/api/types'

const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()

const state = ref<DockerSetupState | null>(null)
const version = ref('')
const unavailable = ref(false)
// A section stays on screen after its change, to show its log.
const setupTouched = ref(false)
const uninstallTouched = ref(false)

const installed = computed(() => !!state.value && (!!state.value.version || state.value.overlay || state.value.configured || state.value.dockerRunning || state.value.helperRunning))
const statusText = computed(() => {
  const s = state.value
  if (!s) return ''
  if (s.ready) return t('apps.docker.statusRunning', { version: s.version })
  return installed.value ? t('apps.docker.statusPartial') : t('apps.docker.statusMissing')
})

async function load() {
  try {
    const res = await api.apps.docker()
    state.value = res.state
    version.value = res.version
    unavailable.value = !res.state.supported
  } catch (err) {
    unavailable.value = true
  }
}

async function changed(which: 'setup' | 'uninstall') {
  if (which === 'setup') setupTouched.value = true
  else uninstallTouched.value = true
  await load()
  emit('changed') // the Docker tab behind follows
}

onMounted(load)
</script>
