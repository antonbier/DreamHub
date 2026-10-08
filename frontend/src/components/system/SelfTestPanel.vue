<!--
  The box self-test (System → Diagnostics; GET/POST /api/v1/system/selftest,
  admin only — hidden for anyone else). One press asks the box, read-only,
  everything DreamHub so far only knew from its test data — the radio
  lists, picons, service names, SerienRecorder, AutoTimer — and rates each
  answer: fine, worth a look, failed, or not applicable. The box's raw
  answers are kept with the report and can be downloaded as a zip (they
  become test data for development). The last report stays until the next
  run.
-->
<template>
  <section v-if="!forbidden" class="mt-5 rounded border border-line bg-tile p-4">
    <div class="flex flex-wrap items-center gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="text-[14px] font-semibold text-text">{{ t('system.selftest.title') }}</h2>
        <p class="mt-0.5 text-[12.5px] text-muted">{{ t('system.selftest.intro') }}</p>
      </div>
      <button
        type="button"
        class="rounded bg-accent px-3.5 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
        :disabled="running"
        @click="run"
      >
        {{ running ? t('system.selftest.running') : report ? t('system.selftest.runAgain') : t('system.selftest.run') }}
      </button>
    </div>

    <p v-if="error" class="mt-3 text-[13px] text-warn" role="status">{{ error }}</p>

    <template v-if="report">
      <div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-[12.5px]">
        <span class="text-faint">{{ t('system.selftest.ranAt', { when: formatDate(report.startedAt) }) }}</span>
        <span v-for="s in summary" :key="s.status" class="flex items-center gap-1.5" :class="s.text">
          <span class="h-1.5 w-1.5 rounded-full" :class="s.dot" />{{ t(`system.selftest.count.${s.status}`, { count: s.count }) }}
        </span>
        <button
          v-if="report.captured.length > 0"
          type="button"
          class="ms-auto rounded px-2 py-1 text-accent hover:bg-tile-2"
          @click="downloadZip"
        >
          {{ t('system.selftest.download', { count: report.captured.length }) }}
        </button>
        <span v-else class="ms-auto text-faint">{{ t('system.selftest.nothingCaptured') }}</span>
      </div>
      <ul class="mt-2 flex flex-col">
        <li v-for="c in report.checks" :key="c.id" class="flex items-start gap-3 border-b border-line-soft py-2 last:border-none">
          <span class="mt-1.5 h-2 w-2 shrink-0 rounded-full" :class="STYLE[c.status].dot" :title="t(`system.selftest.status.${c.status}`)" />
          <span class="min-w-0 flex-1">
            <span class="block text-[13.5px] text-text">{{ t(`system.selftest.check.${c.id}`) }}</span>
            <span class="block break-words text-[12.5px]" :class="STYLE[c.status].text">{{ detail(c) }}</span>
          </span>
          <span class="shrink-0 text-[11.5px] tabular-nums text-faint">{{ c.ms }} ms</span>
        </li>
      </ul>
    </template>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { SelfTestCheck, SelfTestReport } from '@/api/types'

const { t, te, locale } = useI18n()
const { friendly } = useFriendlyError()
const { timeZone } = useDisplayTimeZone()

const STYLE: Record<SelfTestCheck['status'], { dot: string; text: string }> = {
  ok: { dot: 'bg-ok', text: 'text-muted' },
  warn: { dot: 'bg-warn', text: 'text-warn' },
  fail: { dot: 'bg-onair', text: 'text-onair' },
  skip: { dot: 'bg-faint', text: 'text-faint' },
}

const report = ref<SelfTestReport | null>(null)
const running = ref(false)
const error = ref('')
const forbidden = ref(false)

const summary = computed(() =>
  (['ok', 'warn', 'fail', 'skip'] as const)
    .map((status) => ({ status, count: report.value?.checks.filter((c) => c.status === status).length ?? 0, ...STYLE[status] }))
    .filter((s) => s.count > 0),
)

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short', timeZone: timeZone.value }).format(new Date(iso))
}

// The detail line: the result code's text with its params; the kind of
// service and yes/no values are put into words first.
function detail(c: SelfTestCheck): string {
  const params: Record<string, string | number> = {}
  for (const [k, v] of Object.entries(c.params ?? {})) {
    if (k === 'kind') params[k] = t(`dashboard.nowPlaying.kind.${v}`)
    else if (typeof v === 'boolean') params[k] = v ? t('system.selftest.yes') : t('system.selftest.no')
    else params[k] = v === '' ? '—' : v
  }
  const key = `system.selftest.result.${c.code}`
  return te(key) ? t(key, params) : c.code + (params.error ? `: ${params.error}` : '')
}

onMounted(async () => {
  try {
    report.value = (await api.system.selfTest()).report
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) forbidden.value = true
  }
})

async function run() {
  running.value = true
  error.value = ''
  try {
    report.value = (await api.system.runSelfTest()).report
  } catch (err) {
    const code = err instanceof ApiError ? err.code : 'network_error'
    error.value = code === 'selftest_running' ? t('system.selftest.alreadyRunning') : friendly(err).text
  } finally {
    running.value = false
  }
}

async function downloadZip() {
  try {
    const blob = await api.system.selfTestZip()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'dreamhub-selftest.zip'
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  } catch (err) {
    error.value = friendly(err).text
  }
}
</script>
