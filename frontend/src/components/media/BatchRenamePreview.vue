<!--
  Batch rename by the naming scheme (POST /api/v1/media/rename/batch/plan,
  Settings → Recordings): the selected recordings, each before → after,
  plus those left alone and why (already named so, name taken, in use…).
  The source switch re-plans with names completed by TMDB (a film's year,
  an episode's title — exact title matches only). One confirm renames
  every listed recording, all or nothing. Unlike the single-row forms this
  owns its API calls: it's one self-contained flow over a whole selection.
-->
<template>
  <div class="rounded border border-line-soft bg-bg p-3" role="group" :aria-label="t('media.batch.title')">
    <div class="flex flex-wrap items-center gap-3">
      <p class="text-[12px] text-faint">{{ t('plan.eyebrow') }}</p>
      <label class="flex cursor-pointer items-center gap-2 text-[12.5px] text-text">
        <input v-model="folders" type="checkbox" class="rounded" :disabled="busy" @change="load" />
        {{ t('media.batch.folders') }}
      </label>
      <div class="ms-auto flex items-center gap-1 rounded bg-field p-1 text-[12.5px]" role="radiogroup" :aria-label="t('media.batch.source')">
        <button
          v-for="s in sources"
          :key="s.id"
          type="button"
          role="radio"
          :aria-checked="source === s.id"
          class="rounded px-2 py-0.5 disabled:opacity-50"
          :class="source === s.id ? 'bg-tile text-text' : 'text-muted hover:text-text'"
          :disabled="!s.enabled || busy"
          :title="s.enabled ? undefined : t('media.suggest.needsKey.tmdb')"
          @click="setSource(s.id)"
        >
          {{ t(`media.batch.sources.${s.id}`) }}
        </button>
      </div>
    </div>

    <p v-if="busy && !items.length" class="mt-2 text-[13px] text-faint" aria-live="polite">{{ t('common.loading') }}</p>
    <template v-else>
      <p class="mt-2 text-[13.5px] text-text">{{ summary }}</p>
      <ul class="mt-2 flex max-h-[360px] flex-col gap-1 overflow-y-auto font-mono text-[12.5px]">
        <li v-for="it in items" :key="it.path" class="break-all">
          <template v-if="it.status === 'rename'">
            <span class="text-faint line-through">{{ it.before }}</span>
            <span class="text-faint"> → </span>
            <span class="text-accent"><template v-if="it.folder">{{ it.folder }}/</template>{{ it.after }}</span>
          </template>
          <template v-else>
            <span class="text-faint">{{ it.before }}</span>
            <span class="font-sans text-faint"> — {{ reason(it) }}</span>
          </template>
        </li>
      </ul>
    </template>
    <p v-if="newFolders.length" class="mt-2 text-[12.5px] text-muted">
      {{ t('media.batch.newFolders', { n: newFolders.length }, newFolders.length) }}
      <span class="font-mono text-[12px] text-faint">{{ newFolders.map(shortFolder).join(' · ') }}</span>
    </p>
    <p class="mt-2 text-[12px] text-faint">{{ t('media.batch.schemeHint') }}</p>
    <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>

    <div class="mt-3 flex justify-end gap-2">
      <button type="button" class="rounded px-3 py-1.5 text-[13px] text-text hover:bg-field" :disabled="applying" @click="$emit('cancel')">
        {{ t('plan.cancel') }}
      </button>
      <button
        ref="confirmButton"
        type="button"
        class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
        :disabled="!plan || busy || applying"
        @click="confirm"
      >
        {{ applying ? t('plan.applying') : t('media.batch.confirm', { n: renameCount }, renameCount) }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import type { BatchRenameItem, Plan } from '@/api/types'

const props = defineProps<{ paths: string[]; tmdb: boolean }>()
const emit = defineEmits<{ cancel: []; done: [count: number] }>()

const { t } = useI18n()
const { planErrorMessage } = usePlanError()

const sources = computed(() => [
  { id: 'metadata' as const, enabled: true },
  { id: 'tmdb' as const, enabled: props.tmdb },
])
const source = ref<'metadata' | 'tmdb'>('metadata')
// Also sort into folders by the folder schemes (Settings → Recordings).
const folders = ref(false)
const newFolders = ref<string[]>([])
// A new folder shown from its recording folder down ("…/Staffel 05").
function shortFolder(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.slice(-2).join('/')
}
const items = ref<BatchRenameItem[]>([])
const plan = ref<Plan | null>(null)
const busy = ref(false)
const applying = ref(false)
const error = ref('')

const renameCount = computed(() => items.value.filter((i) => i.status === 'rename').length)
const summary = computed(() => {
  const unchanged = items.value.filter((i) => i.status === 'unchanged').length
  const skipped = items.value.filter((i) => i.status === 'skipped').length
  const parts = [t('media.batch.willRename', { n: renameCount.value }, renameCount.value)]
  if (unchanged) parts.push(t('media.batch.unchanged', { n: unchanged }, unchanged))
  if (skipped) parts.push(t('media.batch.skipped', { n: skipped }, skipped))
  return parts.join(' · ')
})

function reason(it: BatchRenameItem) {
  if (it.status === 'unchanged') return t('media.batch.alreadyNamed')
  const key = `error.media.${it.reason}`
  const text = t(key)
  return text === key ? (it.reason ?? '') : text
}

async function load() {
  busy.value = true
  error.value = ''
  plan.value = null
  try {
    const res = await api.media.batchRenamePlan(props.paths, source.value, folders.value)
    items.value = res.items
    plan.value = res.plan ?? null
    newFolders.value = res.folders ?? []
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

function setSource(s: 'metadata' | 'tmdb') {
  if (s === source.value) return
  source.value = s
  void load()
}

async function confirm() {
  if (!plan.value) return
  applying.value = true
  error.value = ''
  try {
    await api.plans.apply(plan.value.id)
    emit('done', renameCount.value)
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    applying.value = false
  }
}

const confirmButton = ref<HTMLButtonElement | null>(null)
onMounted(async () => {
  await load()
  confirmButton.value?.focus()
})
</script>
