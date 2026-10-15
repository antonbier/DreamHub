<!--
  AdminHub's service status (GET /api/v1/admin/services): enigma2, DreamHub
  itself and the box's other services with their state — running since
  when, memory, how often systemd had to restart them. Restart (or Start,
  when a unit is down) goes through the shared preview (PlanPreview): the
  plan's warnings say what a restart interrupts — the TV picture and any
  running recording for enigma2, this page for DreamHub. "Logs" fills the
  log viewer's unit filter below (emit). Reloads after a restart and every
  30 s.
-->
<template>
  <section class="mt-5 rounded-sm border border-line bg-tile p-4">
    <div class="flex items-center gap-3">
      <h2 class="text-[14px] font-semibold text-text">{{ t('admin.services.title') }}</h2>
      <button type="button" class="ms-auto rounded-sm p-1 text-muted hover:bg-tile-2 hover:text-text" :aria-label="t('admin.logs.refresh')" :title="t('admin.logs.refresh')" @click="load">
        <Icon name="refresh" :size="15" />
      </button>
    </div>
    <p v-if="loading && !units.length" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="error" class="mt-2 text-[13px] text-faint">{{ error }}</p>
    <ul v-else class="mt-2 flex flex-col">
      <li v-for="u in units" :key="u.name" class="border-b border-line-soft py-2.5 last:border-none">
        <div class="flex items-center gap-3 below-sm:flex-wrap">
          <span class="h-2 w-2 shrink-0 rounded-full" :class="dotClass(u)" :title="u.active" />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-baseline gap-x-2">
              <span class="font-mono text-[13px] text-text">{{ shortName(u.name) }}</span>
              <span class="truncate text-[12.5px] text-faint">{{ u.description }}</span>
            </div>
            <div class="text-[12.5px]" :class="u.active === 'failed' ? 'text-onair' : 'text-muted'">
              {{ stateLine(u) }}
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-1 below-sm:basis-full below-sm:justify-end">
            <button type="button" class="rounded-sm px-2 py-1 text-[12.5px] text-accent hover:bg-tile-2" @click="$emit('logs', u.name)">
              {{ t('admin.services.logs') }}
            </button>
            <button
              type="button"
              class="rounded-sm border border-line px-2.5 py-1 text-[12.5px] text-text hover:bg-field disabled:opacity-60"
              :disabled="!!pending"
              @click="preview(u)"
            >
              {{ u.active === 'active' ? t('admin.services.restart') : t('admin.services.start') }}
            </button>
          </div>
        </div>
        <div v-if="pending?.name === u.name" class="mt-2">
          <p v-if="pending.done" class="flex items-center gap-1.5 text-[13px] text-ok" role="status">
            <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t(pending.started ? 'admin.services.doneStart' : 'admin.services.done', { name: shortName(u.name) }) }}
          </p>
          <PlanPreview
            v-else-if="pending.plan"
            :plan="pending.plan"
            :confirm-label="u.active === 'active' ? t('admin.services.confirmRestart') : t('admin.services.confirmStart')"
            :busy="pending.applying"
            :error="pending.error"
            @confirm="apply"
            @cancel="pending = null"
          >
            {{ u.active === 'active' ? t('admin.services.previewRestart', { name: shortName(u.name) }) : t('admin.services.previewStart', { name: shortName(u.name) }) }}
          </PlanPreview>
          <p v-else-if="pending.error" class="text-[12.5px] text-warn">{{ pending.error }}</p>
        </div>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import Icon from '@/components/Icon.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { formatBytes } from '@/lib/format'
import { usePlanError } from '@/composables/usePlanError'
import type { Plan, ServiceUnit } from '@/api/types'

defineEmits<{ logs: [unit: string] }>()
const { t, locale } = useI18n()
const { friendly } = useFriendlyError()
const { planErrorMessage } = usePlanError()

const units = ref<ServiceUnit[]>([])
const loading = ref(true)
const error = ref('')

async function load() {
  try {
    units.value = (await api.admin.services()).services
    error.value = ''
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'systemd_unavailable' ? t('admin.services.unavailable') : friendly(err).text
  } finally {
    loading.value = false
  }
}

const shortName = (name: string) => name.replace(/\.service$/, '')

function dotClass(u: ServiceUnit) {
  if (u.active === 'active') return 'bg-ok'
  if (u.active === 'failed') return 'bg-onair'
  if (u.active === 'activating' || u.active === 'deactivating' || u.active === 'reloading') return 'bg-warn'
  return 'bg-faint'
}

// "Running for 2 h · 182 MB · restarted 3× by systemd"
function stateLine(u: ServiceUnit): string {
  const parts: string[] = []
  if (u.active === 'active' && u.since) {
    parts.push(t('admin.services.runningFor', { duration: duration(Date.now() - Date.parse(u.since)) }))
  } else {
    parts.push(t(`admin.services.state.${u.active}`) !== `admin.services.state.${u.active}` ? t(`admin.services.state.${u.active}`) : u.active)
  }
  if (u.memory) parts.push(formatBytes(u.memory, locale.value))
  if (u.restarts > 0) parts.push(t('admin.services.restarts', { count: u.restarts }, u.restarts))
  return parts.join(' · ')
}

// The largest sensible unit: minutes, hours or days.
function duration(ms: number): string {
  const min = Math.max(0, Math.round(ms / 60000))
  const fmt = (value: number, unit: 'minute' | 'hour' | 'day') =>
    new Intl.NumberFormat(locale.value, { style: 'unit', unit, unitDisplay: 'short', maximumFractionDigits: 0 }).format(value)
  if (min < 60) return fmt(min, 'minute')
  if (min < 48 * 60) return fmt(Math.floor(min / 60), 'hour')
  return fmt(Math.floor(min / 1440), 'day')
}

type Pending = { name: string; plan?: Plan; applying?: boolean; done?: boolean; started?: boolean; error?: string }
const pending = ref<Pending | null>(null)

async function preview(u: ServiceUnit) {
  pending.value = { name: u.name }
  try {
    const res = await api.admin.servicePlan(u.name)
    if (pending.value?.name === u.name) pending.value = { name: u.name, plan: res.plan }
  } catch (err) {
    if (pending.value?.name === u.name) pending.value = { name: u.name, error: planErrorMessage(err) }
  }
}

let doneTimer: number | undefined
async function apply() {
  const p = pending.value
  if (!p?.plan) return
  pending.value = { ...p, applying: true, error: undefined }
  try {
    await api.plans.apply(p.plan.id)
    pending.value = { name: p.name, done: true, started: p.plan.action === 'service.start' }
    // systemd restarts without waiting; give it a moment before reading.
    window.setTimeout(load, 1500)
    window.clearTimeout(doneTimer)
    doneTimer = window.setTimeout(() => {
      if (pending.value?.done) pending.value = null
    }, 6000)
  } catch (err) {
    // The plan is used up either way; the button starts a fresh preview.
    pending.value = { name: p.name, error: planErrorMessage(err) }
  }
}

let ticker: number | undefined
onMounted(() => {
  void load()
  ticker = window.setInterval(load, 30_000)
})
onBeforeUnmount(() => {
  window.clearInterval(ticker)
  window.clearTimeout(doneTimer)
})
</script>
