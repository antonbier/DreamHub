<!--
  The one "Updates" summary at the top of the Apps page (docs/APPSHUB_PLAN.md,
  App14): how many packages and containers have a newer version, and "Update
  all" — one plan that lists every container and every package that would
  change, in the order they run (containers first, why is said there), and
  applies them one after the other. A step that fails stops the rest, and the
  result says what was done, what failed and what was not started. While a
  recording runs or is about to start, nothing is updated; the same sentence,
  with the time it works again, shows here and in the refusal.
-->
<template>
  <div v-if="summary && (summary.count > 0 || summary.selfUpdate || phase !== 'idle')" class="mt-4">
    <div v-if="summary.count > 0 || phase !== 'idle'" class="rounded-sm border border-accent bg-tile-2 px-3 py-2.5">
      <div v-if="summary.count > 0" class="flex flex-wrap items-center gap-x-3 gap-y-2">
        <Icon name="refresh" :size="16" class="shrink-0 text-accent" />
        <div class="min-w-0 flex-1 text-[13.5px] text-text">
          <p class="font-semibold">{{ t('updates.title', { count: summary.count }, summary.count) }}</p>
          <p class="text-[12.5px] text-muted">{{ parts }}</p>
          <p v-if="summary.window.closed && phase !== 'error'" class="mt-0.5 text-[12.5px] text-warn">{{ heldSentence(summary.window) }}</p>
        </div>
        <button
          v-if="phase === 'idle'"
          type="button"
          class="tap shrink-0 rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg hover:opacity-90"
          @click="preview"
        >
          {{ t('updates.all') }}
        </button>
      </div>

      <!-- The preview: everything that would change, in the order it runs. -->
      <div v-if="phase === 'loading'" class="mt-3 text-[13px] text-faint">{{ t('updates.working') }}</div>
      <div v-else-if="phase === 'plan' || phase === 'applying'" class="mt-3">
        <PlanPreview :plan="plan!.plan" :confirm-label="t('updates.all')" :busy="phase === 'applying'" :error="error" @confirm="apply" @cancel="close">
          <p>{{ t('updates.intro') }}</p>
          <div v-if="containerSteps.length" class="mt-2">
            <p class="text-[12.5px] font-semibold text-muted">1. {{ t('updates.containers') }}</p>
            <ul class="mt-0.5 flex flex-col gap-0.5 text-[13px]">
              <li v-for="s in containerSteps" :key="s.appId">{{ s.name }}</li>
            </ul>
          </div>
          <div v-if="packageSteps.length" class="mt-2">
            <p class="text-[12.5px] font-semibold text-muted">{{ containerSteps.length ? '2' : '1' }}. {{ t('updates.packages') }}</p>
            <ul class="mt-0.5 flex flex-col gap-0.5 font-mono text-[12.5px]">
              <li v-for="s in packageSteps" :key="s.name" :class="s.change === 'remove' ? 'text-onair' : 'text-text'">
                {{ t('admin.packages.step.' + s.change, { name: s.name, from: s.from ?? '', to: s.to ?? '' }) }}
              </li>
            </ul>
          </div>
          <p v-if="containerSteps.length && packageSteps.length" class="mt-2 text-[12.5px] text-faint">{{ t('updates.why') }}</p>
          <p v-if="plan!.skipped.length" class="mt-2 text-[12.5px] text-warn">
            {{ t('updates.skipped', { names: plan!.skipped.map((s) => `${s.name} (${s.reason})`).join(', ') }) }}
          </p>
          <p v-if="containerSteps.length" class="mt-2 text-[12.5px] text-faint">{{ t('updates.watching') }}</p>
        </PlanPreview>
      </div>

      <!-- Refused (a recording) or failed: the sentence, plus what was done. -->
      <div v-else-if="phase === 'error'" class="mt-3 rounded-sm border border-line-soft bg-bg p-3 text-[13px]" role="alert">
        <p v-for="(line, i) in errorLines" :key="i" class="text-warn">{{ line }}</p>
        <pre v-if="output" class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap wrap-break-word font-mono text-[12px] text-text"><span class="text-faint">{{ t('updates.result.output') }}</span>
{{ output }}</pre>
        <div class="mt-2 text-end">
          <button type="button" class="text-[12.5px] text-accent hover:underline" @click="close">{{ t('admin.crashLogs.close') }}</button>
        </div>
      </div>
      <div v-else-if="phase === 'done'" class="mt-3 flex items-center justify-between gap-3 text-[13px]" role="status">
        <span class="flex items-center gap-1.5 text-ok"><span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('updates.done') }}</span>
        <button type="button" class="text-[12.5px] text-accent hover:underline" @click="close">{{ t('admin.crashLogs.close') }}</button>
      </div>
    </div>
    <p v-if="summary.selfUpdate" class="mt-2 text-[12.5px] text-faint">{{ t('updates.selfNote') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { useUpdateError } from '@/composables/useUpdateError'
import { useUpdates } from '@/composables/useUpdates'
import type { UpdateFailureDetails, UpdatePlan } from '@/api/types'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { summary, reload } = useUpdates()
const { heldSentence, failureLines, updateErrorMessage } = useUpdateError()

type Phase = 'idle' | 'loading' | 'plan' | 'applying' | 'error' | 'done'
const phase = ref<Phase>('idle')
const plan = ref<UpdatePlan | null>(null)
const error = ref('')
const errorLines = ref<string[]>([])
const output = ref('')

const parts = computed(() => {
  const s = summary.value
  if (!s) return ''
  const out: string[] = []
  if (s.packages.length) out.push(t('updates.packagesCount', { count: s.packages.length }, s.packages.length))
  if (s.containers.length) out.push(t('updates.containersCount', { count: s.containers.length }, s.containers.length))
  return out.join(' · ')
})
const containerSteps = computed(() => plan.value?.steps.filter((s) => s.kind === 'container') ?? [])
const packageSteps = computed(() => plan.value?.steps.filter((s) => s.kind === 'package') ?? [])

async function preview() {
  phase.value = 'loading'
  error.value = ''
  try {
    plan.value = await api.apps.updatesPlan()
    phase.value = 'plan'
  } catch (err) {
    showError(err)
  }
}

function showError(err: unknown) {
  output.value = ''
  if (err instanceof ApiError && err.code === 'updates_failed') {
    const d = err.details as unknown as UpdateFailureDetails
    errorLines.value = failureLines(d)
    output.value = d.failed?.find((f) => f.output)?.output ?? ''
  } else {
    errorLines.value = [updateErrorMessage(err)]
  }
  phase.value = 'error'
}

async function apply() {
  if (!plan.value) return
  phase.value = 'applying'
  error.value = ''
  try {
    await api.plans.apply(plan.value.plan.id)
    phase.value = 'done'
  } catch (err) {
    showError(err)
  }
  await reload(true)
  emit('changed') // the tabs behind follow
}

function close() {
  phase.value = 'idle'
  plan.value = null
}

onMounted(() => void reload())
</script>
