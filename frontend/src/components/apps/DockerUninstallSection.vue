<!--
  "Uninstall Docker" inside the Docker settings dialog: a plan with
  preview like every change. It asks one plain question first — also
  delete the app data? — and "keep it" is the default, so apps can come
  back after a reinstall. The preview lists what goes: the containers
  DreamHub created, the two services, the overlay-module loading and the
  Docker files (backend/internal/hubs/apps_docker_setup.go). Recordings
  and the TV software are not touched.
-->
<template>
  <section class="mt-5 border-t border-line-soft pt-4">
    <h3 class="text-[14px] font-semibold text-text">{{ t('apps.uninstall.title') }}</h3>
    <p class="mt-1 text-[13px] text-muted">{{ t('apps.uninstall.intro') }}</p>

    <div class="mt-3">
      <button
        v-if="!asking && !pending"
        type="button"
        class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-onair hover:bg-field"
        @click="asking = true"
      >
        {{ t('apps.uninstall.start') }}
      </button>

      <form v-else-if="asking && !pending" class="flex flex-col gap-2" @submit.prevent="preview">
        <label class="flex items-start gap-2 text-[13px] text-text">
          <input v-model="deleteData" type="checkbox" class="mt-0.5" />
          <span>
            {{ t('apps.uninstall.question') }}
            <span class="block text-[12px] text-faint">{{ t('apps.uninstall.questionHint') }}</span>
          </span>
        </label>
        <div class="flex justify-end gap-2">
          <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="asking = false">{{ t('plan.cancel') }}</button>
          <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg">{{ t('apps.uninstall.preview') }}</button>
        </div>
      </form>

      <p v-else-if="pending && !pending.plan && !pending.error && pending.log === undefined" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
      <PlanPreview
        v-else-if="pending?.plan && pending.log === undefined"
        :plan="pending.plan"
        :confirm-label="t('apps.uninstall.confirm')"
        :busy="pending.applying"
        :error="pending.error"
        danger
        @confirm="apply"
        @cancel="reset"
      >
        <p>{{ t('apps.uninstall.previewIntro') }}</p>
        <ul class="mt-1 flex list-disc flex-col gap-0.5 ps-5 text-[13px]">
          <li v-for="s in pending.steps" :key="s.kind">{{ t('apps.uninstall.step.' + s.kind, stepParams(s)) }}</li>
        </ul>
        <p class="mt-2 text-[12.5px] text-faint">{{ pending.deleteData ? t('apps.uninstall.dataGone') : t('apps.uninstall.dataKept') }}</p>
      </PlanPreview>
      <div v-else-if="pending?.log !== undefined" class="rounded-sm border border-line bg-bg p-3">
        <span class="flex items-center gap-1.5 text-[13px]" :class="pending.ok ? 'text-ok' : 'text-onair'">
          <span class="h-1.5 w-1.5 rounded-full" :class="pending.ok ? 'bg-ok' : 'bg-onair'" />
          {{ pending.ok ? t('apps.uninstall.done') : t('apps.uninstall.failed') }}
        </span>
        <pre v-if="pending.log" class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap wrap-break-word font-mono text-[12px] text-text">{{ pending.log }}</pre>
      </div>
      <div v-else-if="pending?.error" class="flex items-start justify-between gap-3">
        <p class="text-[12.5px] text-warn">{{ pending.error }}</p>
        <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="reset">{{ t('admin.crashLogs.close') }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import type { DockerSetupStep, Plan } from '@/api/types'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { planErrorMessage } = usePlanError()

type Pending = { plan?: Plan; steps: DockerSetupStep[]; deleteData: boolean; applying?: boolean; error?: string; log?: string; ok?: boolean }
const asking = ref(false)
const deleteData = ref(false)
const pending = ref<Pending | null>(null)

function reset() {
  pending.value = null
  asking.value = false
  deleteData.value = false
}

function stepParams(s: DockerSetupStep): Record<string, string> {
  const paths = (s.detail?.paths ?? '').split(',').filter(Boolean)
  return { count: s.detail?.count ?? '', paths: paths.join(', ') }
}

async function preview() {
  const del = deleteData.value
  pending.value = { steps: [], deleteData: del }
  try {
    const res = await api.apps.dockerUninstallPlan(del)
    if (pending.value) pending.value = { plan: res.plan, steps: res.steps, deleteData: del }
  } catch (err) {
    if (pending.value) pending.value = { steps: [], deleteData: del, error: planErrorMessage(err) }
  }
}

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  let failure = ''
  try {
    await api.plans.apply(p.plan.id)
  } catch (err) {
    failure = planErrorMessage(err)
  }
  const res = await api.apps.docker().catch(() => null)
  const run = res?.lastRun
  if (run) {
    pending.value = { steps: p.steps, deleteData: p.deleteData, log: run.log, ok: run.ok }
  } else {
    pending.value = { ...p, applying: false, error: failure || undefined }
  }
  emit('changed')
}
</script>
