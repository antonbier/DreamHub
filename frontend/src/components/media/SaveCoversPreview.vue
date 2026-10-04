<!--
  Save covers (POST /api/v1/media/covers/plan): the posters and backdrops
  enrichment found (TMDB), written next to the selected recordings as
  "<name>.jpg" / "<name>.backdrop.jpg" — what Enigma2's own list and other
  plugins show. Recordings that already have a cover keep it: the preview
  says so, and nothing is ever replaced. One confirm writes every listed
  file, all or nothing. Self-contained like BatchRenamePreview.
-->
<template>
  <div class="rounded border border-line-soft bg-bg p-3" role="group" :aria-label="t('media.covers.title')">
    <p class="text-[12px] text-faint">{{ t('plan.eyebrow') }}</p>

    <p v-if="busy && !items.length" class="mt-2 text-[13px] text-faint" aria-live="polite">{{ t('common.loading') }}</p>
    <template v-else>
      <p class="mt-2 text-[13.5px] text-text">{{ summary }}</p>
      <ul class="mt-2 flex max-h-[360px] flex-col gap-1.5 overflow-y-auto text-[12.5px]">
        <li v-for="it in items" :key="it.path" class="flex items-center gap-2">
          <PosterImage v-if="it.status === 'write'" :path="it.poster" size="w92" :width="20" :height="30" alt="" />
          <span :class="it.status === 'write' ? 'text-text' : 'text-faint'">{{ it.title }}</span>
          <span v-if="it.status === 'write'" class="break-all font-mono text-[12px] text-accent">{{ (it.files ?? []).join(' · ') }}</span>
          <span v-else class="text-faint">— {{ reason(it) }}</span>
        </li>
      </ul>
    </template>
    <p class="mt-2 text-[12px] text-faint">{{ t('media.covers.hint') }}</p>
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
        {{ applying ? t('plan.applying') : t('media.covers.confirm', { n: writeCount }, writeCount) }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import PosterImage from '@/components/enrich/PosterImage.vue'
import type { CoverItem, Plan } from '@/api/types'

const props = defineProps<{ paths: string[] }>()
const emit = defineEmits<{ cancel: []; done: [count: number] }>()

const { t } = useI18n()
const { planErrorMessage } = usePlanError()

const items = ref<CoverItem[]>([])
const plan = ref<Plan | null>(null)
const busy = ref(false)
const applying = ref(false)
const error = ref('')

const writeCount = computed(() => items.value.filter((i) => i.status === 'write').length)
const summary = computed(() => {
  const has = items.value.filter((i) => i.status === 'has_cover').length
  const skipped = items.value.filter((i) => i.status === 'skipped').length
  const parts = [t('media.covers.willWrite', { n: writeCount.value }, writeCount.value)]
  if (has) parts.push(t('media.covers.hasCover', { n: has }, has))
  if (skipped) parts.push(t('media.batch.skipped', { n: skipped }, skipped))
  return parts.join(' · ')
})

function reason(it: CoverItem) {
  if (it.status === 'has_cover') return t('media.covers.alreadyThere')
  const key = `error.media.${it.reason}`
  const text = t(key)
  return text === key ? (it.reason ?? '') : text
}

async function load() {
  busy.value = true
  error.value = ''
  plan.value = null
  try {
    const res = await api.media.saveCoversPlan(props.paths)
    items.value = res.items
    plan.value = res.plan ?? null
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

async function confirm() {
  if (!plan.value) return
  applying.value = true
  error.value = ''
  try {
    await api.plans.apply(plan.value.id)
    emit('done', writeCount.value)
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
