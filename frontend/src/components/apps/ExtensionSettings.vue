<!--
  An extension's settings form (docs/EXTENSIONHUB_PLAN.md, schema 2): the
  extension declares its fields — a device key, a name, a language — and
  keeps the values itself; this is the one generic form for all of them.
  Only what the person changed is sent. A secret is never shown again:
  the field says "saved" and stays empty unless they type a new one.
-->
<template>
  <form class="mt-3 flex flex-col gap-3 rounded-sm border border-line-soft bg-bg p-3" @submit.prevent="save">
    <p v-if="loading" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="fields.length === 0 && !error" class="text-[13px] text-faint">{{ t('extensions.settings.none') }}</p>
    <label v-for="f in fields" :key="f.key" class="flex flex-col gap-1 text-[13px]">
      <span class="text-muted">{{ f.label }}</span>
      <select v-if="f.type === 'select'" v-model="draft[f.key]" class="rounded-sm border border-line bg-field px-2 py-1.5 text-text">
        <option v-for="o in f.options ?? []" :key="o.value" :value="o.value">{{ o.label }}</option>
      </select>
      <input v-else-if="f.type === 'bool'" v-model="flags[f.key]" type="checkbox" class="h-4 w-4 self-start accent-(--dh-accent)" />
      <input
        v-else
        v-model="draft[f.key]"
        :type="f.type === 'secret' ? 'password' : 'text'"
        :placeholder="f.type === 'secret' && f.set ? t('extensions.settings.saved') : ''"
        :autocomplete="f.type === 'secret' ? 'new-password' : 'off'"
        class="rounded-sm border border-line bg-field px-2 py-1.5 text-text"
      />
      <span v-if="f.help" class="text-[12px] text-faint">{{ f.help }}</span>
    </label>
    <p v-if="error" class="text-[12.5px] text-warn" role="alert">{{ error }}</p>
    <p v-if="saved" class="text-[12.5px] text-ok" role="status">{{ t('extensions.settings.done') }}</p>
    <div v-if="fields.length > 0" class="flex justify-end gap-2">
      <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="$emit('close')">{{ t('extensions.settings.close') }}</button>
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || changed.length === 0">
        {{ t('extensions.settings.save') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import { useFriendlyError } from '@/composables/useFriendlyError'
import type { ExtensionSetting } from '@/api/types'

const props = defineProps<{ id: string }>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()

const fields = ref<ExtensionSetting[]>([])
const draft = reactive<Record<string, string>>({})
const flags = reactive<Record<string, boolean>>({})
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const saved = ref(false)

function adopt(list: ExtensionSetting[]) {
  fields.value = list
  for (const f of list) {
    if (f.type === 'bool') flags[f.key] = f.value === 'true'
    else draft[f.key] = f.type === 'secret' ? '' : (f.value ?? '')
  }
}

// What differs from what the extension sent; an empty secret is "keep".
const changed = computed(() =>
  fields.value.filter((f) => {
    if (f.type === 'bool') return flags[f.key] !== (f.value === 'true')
    if (f.type === 'secret') return (draft[f.key] ?? '') !== ''
    return (draft[f.key] ?? '') !== (f.value ?? '')
  }),
)

function message(err: unknown) {
  return err instanceof ApiError && err.code === 'extension_setting_rejected' ? err.message : friendly(err).text
}

async function save() {
  busy.value = true
  error.value = ''
  saved.value = false
  const values: Record<string, string> = {}
  for (const f of changed.value) values[f.key] = f.type === 'bool' ? String(flags[f.key]) : draft[f.key]
  try {
    adopt((await api.extensions.saveSettings(props.id, values)).fields)
    saved.value = true
  } catch (err) {
    error.value = message(err)
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  try {
    adopt((await api.extensions.settings(props.id)).fields)
  } catch (err) {
    error.value = message(err)
  } finally {
    loading.value = false
  }
})
</script>
