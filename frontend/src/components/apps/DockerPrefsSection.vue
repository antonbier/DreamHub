<!--
  The Docker settings that are plain choices (docs/APPSHUB_PLAN.md, App13):
  where new apps keep their data, whether they start with the box, the memory
  limit for a container that states none, and the update check. They are
  ordinary DreamHub settings (admin only); every change is saved at once and
  "Reset" shows only on one that differs from its default.
-->
<template>
  <section class="mt-5 border-t border-line-soft pt-4">
    <h3 class="text-[13px] font-semibold text-muted">{{ t('apps.prefs.title') }}</h3>
    <p v-if="loadError" class="mt-2 text-[12.5px] text-warn">{{ loadError }}</p>
    <div v-else-if="loaded" class="mt-3 flex flex-col gap-4">
      <!-- Storage -->
      <div class="flex flex-col gap-1 text-[13px]">
        <div class="flex items-center gap-2">
          <label for="docker-storage" class="text-muted">{{ t('apps.prefs.storage') }}</label>
          <button v-if="!isDefault('apps.storageRoot')" type="button" class="ms-auto text-[12px] text-accent hover:underline" @click="reset('apps.storageRoot')">{{ t('apps.prefs.reset') }}</button>
        </div>
        <select id="docker-storage" :value="storage" class="rounded-sm border border-line bg-field px-2 py-1.5 text-text" @change="save('apps.storageRoot', ($event.target as HTMLSelectElement).value)">
          <option v-for="c in choices" :key="c.path" :value="c.path">{{ c.label }} — {{ t('apps.storageFree', { size: formatBytes(c.freeBytes, locale) }) }}</option>
        </select>
        <span class="text-[12px] text-faint">{{ t('apps.prefs.storageHint') }}</span>
      </div>

      <!-- Auto-start for new apps -->
      <div class="flex flex-col gap-1 text-[13px]">
        <div class="flex items-center gap-2">
          <label class="flex items-center gap-2 text-text">
            <input type="checkbox" :checked="bool('apps.autostartDefault')" @change="save('apps.autostartDefault', ($event.target as HTMLInputElement).checked)" />
            {{ t('apps.prefs.autostart') }}
          </label>
          <button v-if="!isDefault('apps.autostartDefault')" type="button" class="ms-auto text-[12px] text-accent hover:underline" @click="reset('apps.autostartDefault')">{{ t('apps.prefs.reset') }}</button>
        </div>
        <span class="text-[12px] text-faint">{{ t('apps.prefs.autostartHint') }}</span>
      </div>

      <!-- Memory -->
      <div class="flex flex-col gap-1 text-[13px]">
        <div class="flex items-center gap-2">
          <span class="text-muted">{{ t('apps.prefs.memory') }}</span>
          <button v-if="!isDefault('apps.memoryLimitMB')" type="button" class="ms-auto text-[12px] text-accent hover:underline" @click="resetMemory">{{ t('apps.prefs.reset') }}</button>
        </div>
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-1.5 text-text"><input type="radio" name="docker-mem" :checked="!customMemory" @change="setMemoryAuto" /> {{ t('apps.prefs.memoryAuto') }}</label>
          <label class="flex items-center gap-1.5 text-text"><input type="radio" name="docker-mem" :checked="customMemory" @change="customMemory = true" /> {{ t('apps.prefs.memoryCustom') }}</label>
          <template v-if="customMemory">
            <input v-model="memoryMB" type="number" min="64" max="16384" class="w-24 rounded-sm border border-line bg-field px-2 py-1 text-text" :aria-label="t('apps.prefs.memory')" @change="saveMemory" />
            <span class="text-faint">MB</span>
          </template>
        </div>
        <span class="text-[12px] text-faint">{{ t('apps.prefs.memoryHint') }}</span>
        <span v-if="memoryError" class="text-[12px] text-warn">{{ memoryError }}</span>
      </div>

      <!-- Updates -->
      <div class="flex flex-col gap-1 text-[13px]">
        <div class="flex items-center gap-2">
          <span class="text-muted">{{ t('apps.prefs.updates') }}</span>
          <button v-if="!isDefault('apps.updateCheckHours') || !isDefault('apps.updateNotify')" type="button" class="ms-auto text-[12px] text-accent hover:underline" @click="resetUpdates">{{ t('apps.prefs.reset') }}</button>
        </div>
        <label class="flex items-center gap-2 text-text">
          <input type="checkbox" :checked="bool('apps.updateNotify')" @change="save('apps.updateNotify', ($event.target as HTMLInputElement).checked)" />
          {{ t('apps.prefs.updateNotify') }}
        </label>
        <label class="flex items-center gap-2 text-muted">
          {{ t('apps.prefs.updateEvery') }}
          <select :value="hours" :disabled="!bool('apps.updateNotify')" class="rounded-sm border border-line bg-field px-2 py-1 text-text disabled:opacity-60" @change="save('apps.updateCheckHours', Number(($event.target as HTMLSelectElement).value))">
            <option v-for="h in hourChoices" :key="h" :value="h">{{ t('apps.prefs.hours.' + h) }}</option>
          </select>
        </label>
        <span class="text-[12px] text-faint">{{ t('apps.prefs.updateHint') }}</span>
      </div>
    </div>
    <p v-if="saved" class="mt-2 flex items-center gap-1.5 text-[12.5px] text-ok" role="status"><span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('apps.prefs.saved') }}</p>
    <p v-if="saveError" class="mt-2 text-[12.5px] text-warn" role="alert">{{ saveError }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import { useFriendlyError } from '@/composables/useFriendlyError'
import { formatBytes } from '@/lib/format'
import type { AppStorageChoice, SettingField } from '@/api/types'

const { t, locale } = useI18n()
const { friendly } = useFriendlyError()

const fields = ref<Record<string, SettingField>>({})
const choices = ref<AppStorageChoice[]>([])
const loaded = ref(false)
const loadError = ref('')
const saved = ref(false)
const saveError = ref('')
const customMemory = ref(false)
const memoryMB = ref('')
const memoryError = ref('')
let savedTimer = 0

const hourChoices = [6, 12, 24, 72, 168]
const hours = computed(() => Number(fields.value['apps.updateCheckHours']?.value ?? 24))

const isDefault = (key: string) => fields.value[key]?.isDefault !== false
const bool = (key: string) => fields.value[key]?.value === true
// The place new apps go: the one the storage list marks.
const storage = computed(() => (choices.value.find((c) => c.default) ?? choices.value[0])?.path ?? '')

async function load() {
  try {
    const [view, st] = await Promise.all([api.settings.get(), api.apps.storage()])
    const sec = view.sections.find((s) => s.id === 'apps')
    if (!sec) throw new Error('no apps section')
    fields.value = Object.fromEntries(sec.fields.map((f) => [f.key, f]))
    choices.value = st.choices
    const mb = Number(fields.value['apps.memoryLimitMB']?.value ?? 0)
    customMemory.value = mb > 0
    memoryMB.value = mb > 0 ? String(mb) : '256'
    loaded.value = true
  } catch {
    loadError.value = t('apps.prefs.loadError')
  }
}

async function save(key: string, value: unknown) {
  saveError.value = ''
  memoryError.value = ''
  try {
    await api.settings.update({ [key]: value })
    await load()
    saved.value = true
    window.clearTimeout(savedTimer)
    savedTimer = window.setTimeout(() => (saved.value = false), 2500)
  } catch (err) {
    saveError.value = friendly(err).text || (err instanceof ApiError ? err.message : t('apps.prefs.saveError'))
    await load()
  }
}

const reset = (key: string) => save(key, null)
const setMemoryAuto = () => {
  customMemory.value = false
  return save('apps.memoryLimitMB', null)
}
const resetMemory = () => save('apps.memoryLimitMB', null)
function saveMemory() {
  const n = parseInt(memoryMB.value, 10)
  if (!(n >= 64 && n <= 16384)) {
    memoryError.value = t('apps.prefs.memoryRange')
    return
  }
  return save('apps.memoryLimitMB', n)
}
async function resetUpdates() {
  await save('apps.updateCheckHours', null)
  await save('apps.updateNotify', null)
}

onMounted(load)
</script>
