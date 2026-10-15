<!--
  Automatic updates (docs/APPSHUB_PLAN.md, App14), one switch per kind — here
  for Docker's containers or the Packages tab's packages — and the optional
  "only between HH:MM and HH:MM" that both share. Off by default. Whatever the
  switch, nothing is updated while a recording runs or is about to start;
  the time window only applies to automatic updates. Ordinary DreamHub
  settings (admin only), saved at once.
-->
<template>
  <section :class="embedded ? '' : 'mt-5 border-t border-line-soft pt-4'">
    <h3 v-if="!embedded" class="text-[13px] font-semibold text-muted">{{ t('apps.auto.title') }}</h3>
    <p v-if="loadError" class="mt-2 text-[12.5px] text-warn">{{ loadError }}</p>
    <div v-else-if="loaded" class="flex flex-col gap-3 text-[13px]" :class="embedded ? '' : 'mt-3'">
      <div class="flex flex-col gap-1">
        <label class="flex items-center gap-2 text-text">
          <input type="checkbox" :checked="on" @change="save(key, ($event.target as HTMLInputElement).checked)" />
          {{ t('apps.auto.' + kind) }}
        </label>
        <span class="text-[12px] text-faint">{{ t('apps.auto.' + kind + 'Hint') }}</span>
      </div>
      <div v-if="on" class="flex flex-col gap-1">
        <div class="flex flex-wrap items-center gap-x-2 gap-y-1.5 text-text">
          <label class="flex items-center gap-2">
            <input type="checkbox" :checked="nightOnly" @change="save('apps.updateNightOnly', ($event.target as HTMLInputElement).checked)" />
            {{ t('apps.auto.nightBefore') }}
          </label>
          <input type="time" :value="start" :disabled="!nightOnly" :aria-label="t('apps.auto.from')" class="rounded-sm border border-line bg-field px-2 py-1 text-text disabled:opacity-60" @change="saveTime('apps.updateNightStart', $event)" />
          <span class="text-muted">{{ t('apps.auto.nightAnd') }}</span>
          <input type="time" :value="end" :disabled="!nightOnly" :aria-label="t('apps.auto.to')" class="rounded-sm border border-line bg-field px-2 py-1 text-text disabled:opacity-60" @change="saveTime('apps.updateNightEnd', $event)" />
          <span v-if="t('apps.auto.nightAfter')" class="text-muted">{{ t('apps.auto.nightAfter') }}</span>
        </div>
        <span class="text-[12px] text-faint">{{ t('apps.auto.nightHint') }}</span>
      </div>
    </div>
    <p v-if="saveError" class="mt-2 text-[12.5px] text-warn" role="alert">{{ saveError }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { useUpdates } from '@/composables/useUpdates'
import type { SettingField } from '@/api/types'

// embedded: part of another section (Docker settings' Updates), no heading.
const props = defineProps<{ kind: 'containers' | 'packages'; embedded?: boolean }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()
const { reload: reloadUpdates } = useUpdates()

const fields = ref<Record<string, SettingField>>({})
const loaded = ref(false)
const loadError = ref('')
const saveError = ref('')

const key = computed(() => (props.kind === 'containers' ? 'apps.autoUpdateContainers' : 'apps.autoUpdatePackages'))
const bool = (k: string) => fields.value[k]?.value === true
const str = (k: string) => String(fields.value[k]?.value ?? '')
const on = computed(() => bool(key.value))
const nightOnly = computed(() => bool('apps.updateNightOnly'))
const start = computed(() => str('apps.updateNightStart'))
const end = computed(() => str('apps.updateNightEnd'))

async function load() {
  try {
    const view = await api.settings.get()
    const sec = view.sections.find((s) => s.id === 'apps')
    if (!sec) throw new Error('no apps section')
    fields.value = Object.fromEntries(sec.fields.map((f) => [f.key, f]))
    loaded.value = true
  } catch {
    loadError.value = t('apps.prefs.loadError')
  }
}

async function save(k: string, value: unknown) {
  saveError.value = ''
  try {
    await api.settings.update({ [k]: value })
  } catch (err) {
    saveError.value = friendly(err).text || (err instanceof ApiError ? err.message : t('apps.prefs.saveError'))
  }
  await load()
  void reloadUpdates(true)
}

function saveTime(k: string, e: Event) {
  const v = (e.target as HTMLInputElement).value
  if (v) void save(k, v)
}

onMounted(load)
</script>
