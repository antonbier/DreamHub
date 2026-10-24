<!--
  Lists DreamHub remembers (docs/NETCAST_PLAN.md, N1) and "Compare again":
  the list is read again, the bouquet as it is now, and what differs is
  offered — a channel whose address changed (ticked), one the list no longer
  has, one the list gained (both not ticked). Then the preview as for any
  change. The address of a list is never shown; only the host and path are.
-->
<template>
  <div class="mt-6 border-t border-line-soft pt-4">
    <h2 class="text-[16px] font-semibold">{{ t('channels.iptv.sources.title') }}</h2>
    <p class="mt-1 text-[13px] text-muted">{{ t('channels.iptv.sources.explain') }}</p>
    <p v-if="saved" class="mt-3 rounded-sm border border-ok bg-tile px-4 py-3 text-[13.5px] text-text" role="status">{{ saved }}</p>
    <p v-if="loading" class="mt-2 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="failed" class="mt-2 text-[13.5px] text-muted"><ErrorText /></p>
    <p v-else-if="sources.length === 0" class="mt-2 text-[13.5px] text-muted">{{ t('channels.iptv.sources.none') }}</p>

    <ul v-else class="mt-2 flex flex-col gap-3">
      <li v-for="s in sources" :key="s.id" class="rounded-sm border border-line bg-tile p-3">
        <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span class="text-[14.5px] font-semibold text-text">{{ s.name }}</span>
          <span class="min-w-0 truncate text-[12.5px] text-faint">{{ s.source }}</span>
        </div>
        <p class="text-[13px] text-muted">
          {{ t('channels.iptv.sources.bouquet', { name: s.bouquet }) }} ·
          {{ s.comparedAt ? t('channels.iptv.sources.compared', { when: dateTime(s.comparedAt) }) : t('channels.iptv.sources.neverCompared') }}
        </p>
        <p v-if="s.gone" class="text-[13px] text-warn">{{ t('channels.iptv.sources.gone') }}</p>

        <div v-if="open?.source.id !== s.id" class="mt-2 flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50"
            :disabled="busy || !editor || s.gone"
            @click="compare(s)"
          >
            {{ comparing === s.id ? t('channels.iptv.sources.comparing') : t('channels.iptv.sources.compare') }}
          </button>
          <template v-if="forgetting === s.id">
            <span class="text-[13px] text-text">{{ t('channels.iptv.sources.forgetAsk') }}</span>
            <button type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-onair hover:bg-tile-2" :disabled="busy" @click="forget(s)">{{ t('channels.iptv.sources.forgetYes') }}</button>
            <button type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2" @click="forgetting = 0">{{ t('plan.cancel') }}</button>
          </template>
          <button v-else type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2" :disabled="busy" @click="forgetting = s.id">
            {{ t('channels.iptv.sources.forget') }}
          </button>
        </div>
        <p v-if="error && errorFor === s.id" class="mt-2 text-[13px] text-warn" role="alert">{{ error }}</p>

        <!-- the result of the comparison -->
        <div v-if="open?.source.id === s.id" class="mt-3">
          <p v-if="open.items.length === 0" class="text-[13.5px] text-text">{{ t('channels.iptv.sources.nothing', { n: number(open.listSize) }) }}</p>
          <template v-else-if="!plan">
            <section v-for="a in ACTIONS" :key="a" v-show="itemsOf(a).length > 0" class="mb-3">
              <h3 class="text-[13.5px] font-semibold text-text">{{ t(`channels.iptv.sources.action.${a}`) }} ({{ number(itemsOf(a).length) }})</h3>
              <p class="text-[12.5px] text-faint">{{ t(`channels.iptv.sources.hint.${a}`) }}</p>
              <ul class="mt-1 divide-y divide-line-soft rounded-sm border border-line bg-bg">
                <li v-for="it in itemsOf(a)" :key="it.id">
                  <label class="flex cursor-pointer items-center gap-3 px-3 py-2 text-[14px] hover:bg-tile-2">
                    <input type="checkbox" class="h-4 w-4 shrink-0" :checked="picked.has(it.id)" @change="toggle(it.id)" />
                    <span class="min-w-0 grow">
                      <span class="block truncate text-text">{{ it.oldName ?? it.name }}</span>
                      <span v-if="it.group || it.host" class="block truncate text-[12.5px] text-faint">{{ [it.group, it.host].filter(Boolean).join(' · ') }}</span>
                    </span>
                    <span v-if="it.hasLogin" class="shrink-0 rounded-sm border border-line px-1 text-[11px] text-muted">{{ t('channels.iptv.login') }}</span>
                  </label>
                </li>
              </ul>
            </section>
          </template>

          <PlanPreview
            v-if="plan"
            :plan="plan"
            :confirm-label="t('channels.iptv.sources.confirm')"
            :busy="busy"
            :error="error"
            @confirm="apply"
            @cancel="cancelPlan"
          >
            <p v-for="(c, n) in plan.changes.slice(0, PREVIEW_LINES)" :key="n">{{ describeChange(c) }}</p>
            <p v-if="plan.changes.length > PREVIEW_LINES" class="text-muted">{{ t('channels.iptv.moreChanges', { n: number(plan.changes.length - PREVIEW_LINES) }) }}</p>
            <p class="mt-2 text-[12.5px] text-faint">{{ t('channels.iptv.backupNote') }}</p>
          </PlanPreview>

          <div v-else class="mt-2 flex flex-wrap items-center gap-2">
            <button
              v-if="open.items.length > 0"
              type="button"
              class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50"
              :disabled="busy || picked.size === 0"
              @click="review"
            >
              {{ t('channels.iptv.sources.review') }}
            </button>
            <button type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2" @click="close">{{ t('channels.iptv.sources.close') }}</button>
            <span v-if="open.items.length > 0" class="ms-auto text-[13px] text-text" aria-live="polite">{{ t('channels.iptv.selected', { n: number(picked.size) }, picked.size) }}</span>
            <p v-if="error && errorFor === s.id" class="w-full text-[13px] text-warn" role="alert">{{ error }}</p>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ErrorText from '@/components/ErrorText.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { useChannelChange } from '@/components/channels/useChannelChange'
import { usePlanError } from '@/composables/usePlanError'
import type { IptvCompare, IptvCompareItem, IptvSource, Plan } from '@/api/types'

defineProps<{ editor: boolean }>()
const emit = defineEmits<{ done: [] }>()

const { t, n: formatNumber, locale } = useI18n()
const { planErrorMessage } = usePlanError()
const { describeChange } = useChannelChange()

const ACTIONS = ['update', 'gone', 'new'] as const
const PREVIEW_LINES = 12

const number = (v: number) => formatNumber(v)
const dateTime = (iso: string) => new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso))

const sources = ref<IptvSource[]>([])
const loading = ref(true)
const failed = ref(false)
const busy = ref(false)
const comparing = ref(0)
const forgetting = ref(0)
const error = ref('')
const errorFor = ref(0)
const saved = ref('')

const open = ref<IptvCompare | null>(null)
const picked = ref(new Set<number>())
const plan = ref<Plan | null>(null)

const itemsOf = (a: IptvCompareItem['action']) => open.value?.items.filter((i) => i.action === a) ?? []

async function load() {
  loading.value = true
  failed.value = false
  try {
    sources.value = (await api.channels.iptvSources()).sources
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}

function fail(id: number, err: unknown) {
  errorFor.value = id
  error.value = planErrorMessage(err)
}

async function compare(s: IptvSource) {
  busy.value = true
  comparing.value = s.id
  error.value = ''
  saved.value = ''
  try {
    const res = await api.channels.compareIptv(s.id)
    open.value = res
    plan.value = null
    // A changed address is the point of comparing: ticked. The rest is up to the person.
    picked.value = new Set(res.items.filter((i) => i.action === 'update').map((i) => i.id))
    await load()
  } catch (err) {
    fail(s.id, err)
  } finally {
    busy.value = false
    comparing.value = 0
  }
}

function toggle(id: number) {
  const next = new Set(picked.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  picked.value = next
}

function close() {
  open.value = null
  plan.value = null
  error.value = ''
}

function cancelPlan() {
  plan.value = null
  error.value = ''
}

async function review() {
  const o = open.value
  if (!o) return
  busy.value = true
  error.value = ''
  try {
    plan.value = (await api.channels.planIptvCompare(o.source.id, o.token, [...picked.value].sort((a, b) => a - b))).plan
  } catch (err) {
    fail(o.source.id, err)
  } finally {
    busy.value = false
  }
}

async function apply() {
  const p = plan.value
  const o = open.value
  if (!p || !o) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(p.id)
    saved.value = t('channels.iptv.sources.saved')
    close()
    emit('done')
    await load()
  } catch (err) {
    fail(o.source.id, err)
  } finally {
    busy.value = false
  }
}

async function forget(s: IptvSource) {
  busy.value = true
  error.value = ''
  try {
    await api.channels.forgetIptv(s.id)
    forgetting.value = 0
    await load()
  } catch (err) {
    fail(s.id, err)
  } finally {
    busy.value = false
  }
}

defineExpose({ reload: load })
onMounted(load)
</script>
