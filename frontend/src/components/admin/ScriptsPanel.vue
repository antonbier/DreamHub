<!--
  AdminHub's script library (docs/ADMINHUB_PLAN.md, A3, #47): scripts
  uploaded here run only by hand, after the usual preview-and-confirm —
  there's nothing to diff for an arbitrary script, so the preview is just
  "run script X", but it still can't be skipped. Running records the exit
  code and output (admin_script_runs), shown right after. AutomationHub
  (later) wires the same stored scripts into rules instead of keeping its
  own copies.
-->
<template>
  <section class="mt-5 rounded-sm border border-line bg-tile p-4">
    <div class="flex items-center gap-3">
      <h2 class="text-[14px] font-semibold text-text">{{ t('admin.scripts.title') }}</h2>
      <button type="button" class="ms-auto rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60" :disabled="uploading" @click="fileInput?.click()">
        {{ t('admin.scripts.upload') }}
      </button>
      <input ref="fileInput" type="file" class="hidden" @change="onFileChosen" />
    </div>
    <p v-if="uploadError" class="mt-2 text-[12.5px] text-warn">{{ uploadError }}</p>

    <p v-if="loading && scripts.length === 0" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="scripts.length === 0" class="mt-2 text-[13px] text-faint">{{ t('admin.scripts.empty') }}</p>
    <ul v-else class="mt-2 flex flex-col">
      <li v-for="s in scripts" :key="s.id" class="border-b border-line-soft py-2.5 last:border-none">
        <div class="flex items-center gap-3 below-sm:flex-wrap">
          <div class="min-w-0 flex-1">
            <span class="block truncate font-mono text-[13px] text-text">{{ s.name }}</span>
            <span class="block text-[12px] text-faint">{{ formatDate(s.uploadedAt) }}</span>
          </div>
          <div class="flex shrink-0 items-center gap-1 below-sm:basis-full below-sm:justify-end">
            <template v-if="confirmingDelete === s.id">
              <span class="text-[12.5px] text-muted">{{ t('admin.scripts.deleteConfirm') }}</span>
              <button type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-onair hover:bg-tile-2" @click="remove(s.id)">{{ t('admin.scripts.deleteYes') }}</button>
              <button type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-text hover:bg-tile-2" @click="confirmingDelete = null">{{ t('plan.cancel') }}</button>
            </template>
            <template v-else>
              <button type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-text hover:bg-tile-2" :disabled="!!pending" @click="confirmingDelete = s.id">
                {{ t('admin.scripts.delete') }}
              </button>
              <button type="button" class="rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60" :disabled="!!pending" @click="preview(s)">
                {{ t('admin.scripts.run') }}
              </button>
            </template>
          </div>
        </div>
        <div v-if="pending?.id === s.id" class="mt-2">
          <PlanPreview
            v-if="pending.plan && !pending.result"
            :plan="pending.plan"
            :confirm-label="t('admin.scripts.confirmRun')"
            :busy="pending.applying"
            :error="pending.error"
            @confirm="apply"
            @cancel="pending = null"
          >
            {{ t('admin.scripts.previewRun', { name: s.name }) }}
          </PlanPreview>
          <p v-else-if="pending.error" class="text-[12.5px] text-warn">{{ pending.error }}</p>
          <div v-else-if="pending.result" class="rounded-sm border border-line bg-bg p-3">
            <div class="flex items-center justify-between gap-3">
              <span class="flex items-center gap-1.5 text-[13px]" :class="pending.result.exitCode === 0 ? 'text-ok' : 'text-onair'">
                <span class="h-1.5 w-1.5 rounded-full" :class="pending.result.exitCode === 0 ? 'bg-ok' : 'bg-onair'" />
                {{ t('admin.scripts.exitCode', { code: pending.result.exitCode }) }}
              </span>
              <button type="button" class="text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
            </div>
            <pre v-if="pending.result.output" class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap wrap-break-word font-mono text-[12px] text-text">{{ pending.result.output }}</pre>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { AdminScript, Plan, ScriptRun } from '@/api/types'

const { t, locale } = useI18n()
const { planErrorMessage } = usePlanError()
const { timeZone } = useDisplayTimeZone()

const scripts = ref<AdminScript[]>([])
const loading = ref(true)

async function load() {
  try {
    scripts.value = (await api.admin.scripts()).scripts
  } finally {
    loading.value = false
  }
}

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

// --- upload -------------------------------------------------------------

const fileInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const uploadError = ref('')

async function onFileChosen(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (fileInput.value) fileInput.value.value = ''
  if (!file) return
  uploadError.value = ''
  uploading.value = true
  try {
    const content = await file.text()
    await api.admin.uploadScript(file.name, content)
    await load()
  } catch {
    uploadError.value = t('admin.scripts.uploadFailed')
  } finally {
    uploading.value = false
  }
}

// --- delete ---------------------------------------------------------------

const confirmingDelete = ref<number | null>(null)

async function remove(id: number) {
  confirmingDelete.value = null
  await api.admin.deleteScript(id)
  await load()
}

// --- run: preview, confirm, show the result --------------------------------

type Pending = { id: number; plan?: Plan; applying?: boolean; error?: string; result?: ScriptRun }
const pending = ref<Pending | null>(null)

async function preview(s: AdminScript) {
  pending.value = { id: s.id }
  try {
    const res = await api.admin.runScriptPlan(s.id)
    if (pending.value?.id === s.id) pending.value = { id: s.id, plan: res.plan }
  } catch (err) {
    if (pending.value?.id === s.id) pending.value = { id: s.id, error: planErrorMessage(err) }
  }
}

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    const runs = (await api.admin.scriptRuns(p.id)).runs
    pending.value = { id: p.id, result: runs[0] }
  } catch (err) {
    pending.value = { id: p.id, error: planErrorMessage(err) }
  }
}

onMounted(load)
</script>
