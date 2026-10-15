<!--
  "Add container" (docs/APPSHUB_PLAN.md, App12; admin only): paste a compose
  file, name an image, or import an Unraid template. The server reads it
  (nothing is downloaded, nothing is created yet), refuses what the helper
  would refuse and says why, and the same install form as the catalog's
  opens for the rest. A standing notice says whose responsibility it is.
-->
<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="emit('close')">
    <div class="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-y-auto rounded-sm border border-line bg-bg p-4" role="dialog" aria-modal="true" :aria-label="t('apps.custom.title')">
      <div class="flex items-center justify-between">
        <h3 class="text-[14px] font-semibold text-text">{{ t('apps.custom.title') }}</h3>
        <button type="button" class="text-[12.5px] text-muted hover:text-text" @click="emit('close')">{{ t('apps.custom.close') }}</button>
      </div>

      <!-- Step 1: what to add -->
      <form v-if="!parsed" class="mt-3 flex flex-col gap-3" @submit.prevent="parse">
        <p class="text-[13px] text-muted">{{ t('apps.custom.intro') }}</p>
        <p class="rounded-sm border border-line-soft bg-tile p-2.5 text-[12.5px] text-muted">{{ t('apps.custom.responsibility') }}</p>

        <div class="flex gap-1" role="tablist">
          <button
            v-for="k in kinds"
            :key="k"
            type="button"
            role="tab"
            :aria-selected="kind === k"
            class="rounded-sm px-2.5 py-1 text-[12.5px]"
            :class="kind === k ? 'bg-accent font-semibold text-bg' : 'text-muted hover:bg-field'"
            @click="kind = k"
          >
            {{ t('apps.custom.kind.' + k) }}
          </button>
        </div>

        <label v-if="kind === 'image'" class="flex flex-col gap-1 text-[13px]">
          <span class="text-muted">{{ t('apps.custom.imageLabel') }}</span>
          <input v-model="text" type="text" spellcheck="false" autocomplete="off" placeholder="ghcr.io/example/tool:1.2" class="rounded-sm border border-line bg-field px-2 py-1.5 font-mono text-[13px] text-text" />
          <span class="text-[12px] text-faint">{{ t('apps.custom.imageHelp') }}</span>
        </label>
        <label v-else class="flex flex-col gap-1 text-[13px]">
          <span class="text-muted">{{ kind === 'compose' ? t('apps.custom.composeLabel') : t('apps.custom.unraidLabel') }}</span>
          <textarea
            v-model="text"
            rows="12"
            spellcheck="false"
            class="rounded-sm border border-line bg-field px-2 py-1.5 font-mono text-[12px] text-text"
            :placeholder="placeholders[kind]"
          />
          <span class="text-[12px] text-faint">
            {{ kind === 'compose' ? t('apps.custom.composeHelp', { example: '${PORT:-8080}' }) : t('apps.custom.unraidHelp') }}
          </span>
        </label>

        <ul v-if="issues.length > 0" class="flex flex-col gap-1 text-[12.5px] text-warn" role="alert">
          <li v-for="(i, n) in issues" :key="n">{{ issueText(i) }}</li>
        </ul>
        <p v-else-if="error" class="text-[12.5px] text-warn" role="alert">{{ error }}</p>

        <div class="flex justify-end gap-2">
          <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="emit('close')">{{ t('plan.cancel') }}</button>
          <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || text.trim() === ''">
            {{ busy ? t('apps.custom.checking') : t('apps.custom.continue') }}
          </button>
        </div>
      </form>

      <!-- Step 2: the install form -->
      <div v-else class="mt-3 flex flex-col gap-3">
        <p class="text-[12.5px] text-faint">{{ t('apps.custom.responsibilityShort') }}</p>
        <p v-if="parsed.description" class="text-[13px] text-muted">{{ parsed.description }}</p>
        <p v-if="parsed.note" class="text-[12.5px] text-muted">{{ parsed.note }}</p>
        <p v-if="parsed.services.length > 1" class="text-[12.5px] text-faint">
          {{ t('apps.custom.contains', { services: parsed.services.map((s) => s.image).join(', ') }) }}
        </p>
        <details class="text-[12.5px] text-muted">
          <summary class="cursor-pointer font-semibold">{{ t('apps.custom.hostNetwork') }}</summary>
          <p class="mt-1 text-faint">{{ t('apps.custom.hostNetworkHint') }}</p>
        </details>
        <div v-if="parsed.notes.length > 0" class="rounded-sm border border-line-soft p-2.5 text-[12.5px]">
          <p class="font-semibold text-muted">{{ t('apps.custom.notesTitle') }}</p>
          <ul class="mt-1 list-disc ps-5 text-muted">
            <li v-for="(n, i) in parsed.notes" :key="i">{{ issueText(n) }}</li>
          </ul>
        </div>
        <AppInstallForm :source="{ kind: 'custom', app: parsed }" @done="onDone" @cancel="parsed = null" />
        <button v-if="!installed" type="button" class="self-start text-[12.5px] text-muted hover:text-text" @click="parsed = null">← {{ t('apps.custom.back') }}</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import AppInstallForm from '@/components/apps/AppInstallForm.vue'
import { useAppForm } from '@/composables/useAppForm'
import type { ComposeIssue, ParsedApp } from '@/api/types'

const props = defineProps<{ initial?: { kind: 'compose' | 'image' | 'unraid'; text: string } }>()
const emit = defineEmits<{ close: []; done: [] }>()

const { t, locale } = useI18n()
const { issueText, failure } = useAppForm()

const kinds = ['compose', 'image', 'unraid'] as const
const placeholders = { compose: 'services:\n  app:\n    image: …', image: 'ghcr.io/example/tool:1.2', unraid: '<Container version="2">…' }
const kind = ref<(typeof kinds)[number]>(props.initial?.kind ?? 'compose')
const text = ref(props.initial?.text ?? '')
const busy = ref(false)
const error = ref('')
const issues = ref<ComposeIssue[]>([])
const parsed = ref<ParsedApp | null>(null)
const installed = ref(false)

async function parse() {
  busy.value = true
  error.value = ''
  issues.value = []
  try {
    parsed.value = (await api.apps.parseCustom(kind.value, text.value, locale.value)).app
  } catch (err) {
    const f = failure(err)
    error.value = f.message
    issues.value = f.issues
  } finally {
    busy.value = false
  }
}

function onDone() {
  installed.value = true
  emit('done')
  window.setTimeout(() => emit('close'), 1800)
}

// "Add as container" from a search result opens with the image filled in.
onMounted(() => {
  if (props.initial?.kind === 'image' && props.initial.text) void parse()
})
</script>
