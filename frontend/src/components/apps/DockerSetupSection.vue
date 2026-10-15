<!--
  "Set up Docker" inside the Docker settings dialog
  (backend/internal/dockersetup): shown while Docker isn't running on the
  box. The preview lists exactly what's still missing — the overlay
  kernel module matching this box's kernel, the pinned Docker release
  (download, checksum), its config on /data, the two services — and
  applying shows the setup's own log.
-->
<template>
  <section class="mt-5 border-t border-line-soft pt-4">
    <h3 class="text-[14px] font-semibold text-text">{{ t('apps.docker.setUpTitle') }}</h3>
    <p class="mt-1 text-[13px] text-muted">{{ t('apps.docker.setUpIntro') }}</p>
    <p class="mt-1 text-[12.5px] text-faint">{{ t('apps.docker.where', { version, free: formatSize(state.freeBytes) }) }}</p>
    <p v-if="unavailable" class="mt-2 text-[12.5px] text-faint">{{ t('apps.docker.unavailable') }}</p>

    <div v-else class="mt-3">
      <button
        v-if="!pending"
        type="button"
        class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg"
        @click="preview"
      >
        {{ t('apps.docker.setUp') }}
      </button>
      <p v-else-if="!pending.plan && !pending.error && !pending.log" class="text-[13px] text-faint">{{ t('common.loading') }}</p>
      <PlanPreview
        v-else-if="pending.plan && pending.log === undefined"
        :plan="pending.plan"
        :confirm-label="t('apps.docker.confirm')"
        :busy="pending.applying"
        :error="pending.error"
        @confirm="apply"
        @cancel="pending = null"
      >
        <p>{{ t('apps.docker.previewIntro') }}</p>
        <ul class="mt-1 flex list-disc flex-col gap-0.5 ps-5 text-[13px]">
          <li v-for="s in pending.steps" :key="s.kind">{{ t('apps.docker.step.' + s.kind + stepVariant(s), stepParams(s)) }}</li>
        </ul>
        <p class="mt-2 text-[12.5px] text-faint">{{ t('apps.docker.takesAWhile') }}</p>
      </PlanPreview>
      <div v-else-if="pending.log !== undefined" class="rounded-sm border border-line bg-bg p-3">
        <span class="flex items-center gap-1.5 text-[13px]" :class="pending.ok ? 'text-ok' : 'text-onair'">
          <span class="h-1.5 w-1.5 rounded-full" :class="pending.ok ? 'bg-ok' : 'bg-onair'" />
          {{ pending.ok ? t('apps.docker.done') : t('apps.docker.failed') }}
        </span>
        <pre v-if="pending.log" class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap wrap-break-word font-mono text-[12px] text-text">{{ pending.log }}</pre>
        <button v-if="!pending.ok" type="button" class="mt-2 text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('apps.docker.retry') }}</button>
      </div>
      <div v-else-if="pending.error" class="flex items-start justify-between gap-3">
        <p class="text-[12.5px] text-warn">{{ pending.error }}</p>
        <button type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" @click="pending = null">{{ t('admin.crashLogs.close') }}</button>
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
import type { DockerSetupState, DockerSetupStep, Plan } from '@/api/types'

const props = defineProps<{ state: DockerSetupState; version: string; unavailable: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t, locale } = useI18n()
const { planErrorMessage } = usePlanError()

type Pending = { plan?: Plan; steps: DockerSetupStep[]; applying?: boolean; error?: string; log?: string; ok?: boolean }
const pending = ref<Pending | null>(null)

function formatSize(bytes: number): string {
  return new Intl.NumberFormat(locale.value, { style: 'unit', unit: 'gigabyte', maximumFractionDigits: 1 }).format(bytes / 2 ** 30)
}

// The overlay step reads differently depending on where the module comes from.
function stepVariant(s: DockerSetupStep): string {
  return s.kind === 'overlay' ? '.' + (s.detail?.source ?? 'installed') : ''
}

function stepParams(s: DockerSetupStep): Record<string, string> {
  return { version: s.detail?.version ?? props.version, dataRoot: s.detail?.dataRoot ?? '', to: s.detail?.to ?? '' }
}

async function preview() {
  pending.value = { steps: [] }
  try {
    const res = await api.apps.dockerSetupPlan()
    if (pending.value) pending.value = { plan: res.plan, steps: res.steps }
  } catch (err) {
    if (pending.value) pending.value = { steps: [], error: planErrorMessage(err) }
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
    pending.value = { steps: p.steps, log: run.log, ok: run.ok }
  } else {
    pending.value = { ...p, applying: false, error: failure || undefined }
  }
  emit('changed')
}
</script>
