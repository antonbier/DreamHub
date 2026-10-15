<!--
  AppsHub's "Extensions" tab (docs/EXTENSIONHUB_PLAN.md, E1, #46 —
  unchanged by the AppsHub rename, docs/APPSHUB_PLAN.md, App1): one card
  per registered extension — installing five extensions grows the
  navigation by zero entries, not five. Each card is a handful of
  status/meter/list/action widgets an extension's own separate process
  describes as plain data (backend/internal/extension); this is the one
  generic renderer for all of them, so an extension never ships frontend
  code. An action widget's button goes through the same preview-and-confirm
  flow (PlanPreview) every built-in hub's own actions do — but since an
  extension owns no i18n key in DreamHub's own locale files, its preview
  text is the plan's own `summary` field, shown verbatim rather than
  translated (contrast every other hub's panel, which builds its preview
  text from a local i18n key).
-->
<template>
  <div>
    <p v-if="loading" class="mt-4 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="mt-4 text-[13px] text-faint">{{ error }}</p>
    <p v-else-if="cards.length === 0" class="mt-4 text-[13px] text-faint">{{ t('extensions.empty') }}</p>

    <div v-else class="mt-5 flex flex-col gap-4">
      <section v-for="card in cards" :key="card.id" class="rounded-sm border border-line bg-tile p-4">
        <div class="flex items-center gap-2">
          <Icon :name="card.icon || 'extension'" :size="18" class="text-muted" />
          <h2 class="text-[14px] font-semibold text-text">{{ card.name }}</h2>
          <span v-if="card.status !== 'available'" class="h-1.5 w-1.5 rounded-full" :class="card.status === 'degraded' ? 'bg-warn' : 'bg-faint'" />
          <button
            v-if="card.settings"
            type="button"
            class="ms-auto rounded-sm px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2"
            :aria-expanded="settingsOpen === card.id"
            @click="settingsOpen = settingsOpen === card.id ? null : card.id"
          >
            {{ t('extensions.settings.open') }}
          </button>
        </div>
        <ExtensionSettings v-if="settingsOpen === card.id" :id="card.id" @close="settingsOpen = null" />

        <ul v-if="card.problems.length > 0" class="mt-1.5 flex flex-col gap-1 text-[12.5px] text-muted">
          <li v-for="(p, i) in card.problems" :key="i">
            {{ describe(p) }}
            <span v-if="describeFix(p)" class="text-faint">{{ ' — ' + describeFix(p) }}</span>
          </li>
        </ul>

        <div v-if="card.widgets.length > 0" class="mt-3 flex flex-col gap-3">
          <template v-for="(w, i) in card.widgets" :key="i">
            <!-- status -->
            <div v-if="w.type === 'status'" class="flex items-center gap-2 text-[13.5px]">
              <span class="h-2 w-2 shrink-0 rounded-full" :class="toneDot(w.tone)" />
              <span class="text-muted">{{ w.label }}</span>
              <span class="ms-auto font-medium text-text">{{ w.value }}</span>
            </div>

            <!-- meter -->
            <div v-else-if="w.type === 'meter'">
              <div class="flex items-baseline justify-between text-[12.5px] text-muted">
                <span>{{ w.label }}</span>
                <span>{{ w.current }}{{ w.unit }} / {{ w.max }}{{ w.unit }}</span>
              </div>
              <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-tile-2">
                <div class="h-full rounded-full bg-accent" :style="{ width: meterPercent(w) + '%' }" />
              </div>
            </div>

            <!-- list -->
            <ul v-else-if="w.type === 'list'" class="flex flex-col gap-1">
              <li v-for="(row, j) in w.rows" :key="j" class="flex items-center gap-2 text-[13px]">
                <Icon v-if="row.icon" :name="row.icon" :size="14" class="shrink-0 text-faint" />
                <span class="text-muted">{{ row.label }}</span>
                <span class="ms-auto text-text">{{ row.value }}</span>
              </li>
            </ul>

            <!-- action -->
            <div v-else-if="w.type === 'action'">
              <button
                type="button"
                class="rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
                :disabled="!!pending"
                @click="preview(card.id, w)"
              >
                {{ w.buttonLabel || w.label }}
              </button>
              <div v-if="pending?.cardId === card.id && pending.actionId === w.actionId" class="mt-2">
                <p v-if="pending.done" class="flex items-center gap-1.5 text-[13px] text-ok" role="status">
                  <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('extensions.done') }}
                </p>
                <PlanPreview
                  v-else-if="pending.plan"
                  :plan="pending.plan"
                  :confirm-label="t('extensions.confirm')"
                  :busy="pending.applying"
                  :error="pending.error"
                  @confirm="apply"
                  @cancel="pending = null"
                >
                  {{ pending.plan.summary }}
                </PlanPreview>
                <p v-else-if="pending.error" class="text-[12.5px] text-warn">{{ pending.error }}</p>
              </div>
            </div>
          </template>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import ExtensionSettings from '@/components/apps/ExtensionSettings.vue'
import { useProblem } from '@/composables/useProblem'
import { usePlanError } from '@/composables/usePlanError'
import type { ExtensionCard, ExtensionWidget, Plan } from '@/api/types'

const { t } = useI18n()
const { describe, describeFix } = useProblem()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const cards = ref<ExtensionCard[]>([])
const settingsOpen = ref<string | null>(null)
const loading = ref(true)
const error = ref('')

async function load() {
  try {
    cards.value = (await api.extensions.list()).extensions
    error.value = ''
  } catch (err) {
    error.value = friendly(err).text
  } finally {
    loading.value = false
  }
}

function toneDot(tone?: string) {
  if (tone === 'good') return 'bg-ok'
  if (tone === 'bad') return 'bg-onair'
  return 'bg-faint'
}

function meterPercent(w: ExtensionWidget): number {
  if (!w.max) return 0
  return Math.max(0, Math.min(100, (100 * (w.current ?? 0)) / w.max))
}

type Pending = { cardId: string; actionId: string; plan?: Plan; applying?: boolean; done?: boolean; error?: string }
const pending = ref<Pending | null>(null)

async function preview(cardId: string, w: ExtensionWidget) {
  if (!w.actionId) return
  pending.value = { cardId, actionId: w.actionId }
  try {
    const res = await api.extensions.planAction(cardId, w.actionId)
    if (pending.value?.cardId === cardId && pending.value.actionId === w.actionId) {
      pending.value = { cardId, actionId: w.actionId, plan: res.plan }
    }
  } catch (err) {
    if (pending.value?.cardId === cardId && pending.value.actionId === w.actionId) {
      pending.value = { cardId, actionId: w.actionId, error: planErrorMessage(err) }
    }
  }
}

async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    pending.value = { cardId: p.cardId, actionId: p.actionId, done: true }
    window.setTimeout(load, 500)
    window.setTimeout(() => {
      if (pending.value?.done) pending.value = null
    }, 4000)
  } catch (err) {
    pending.value = { cardId: p.cardId, actionId: p.actionId, error: planErrorMessage(err) }
  }
}

onMounted(load)
</script>
