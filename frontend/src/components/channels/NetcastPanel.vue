<!--
  Netcast's sources on the box (docs/NETCAST_PLAN.md, N3): the playlists,
  APIs and guide sources the Gemini Netcast plugin has set up, from its own
  files, and a way to add, change or remove one. Every change is a preview
  first; DreamHub keeps a copy of the old file next to it and reads the file
  back. A login is never shown, only that a source has one. A box whose
  plugin cannot write the files yet shows the sources and says so.
-->
<template>
  <section class="mt-4">
    <p class="text-[13.5px] text-muted">{{ t('channels.netcast.explain') }}</p>
    <p v-if="saved" class="mt-3 rounded-sm border border-ok bg-tile px-4 py-3 text-[13.5px] text-text" role="status">{{ saved }}</p>
    <p v-if="loading" class="mt-3 text-[13.5px] text-muted">{{ t('common.loading') }}</p>
    <p v-else-if="failed" class="mt-3 text-[13.5px] text-muted"><ErrorText /></p>
    <p v-else-if="data && !data.available" class="mt-3 rounded-sm border border-line bg-tile px-4 py-3 text-[13.5px] text-muted">{{ t('channels.netcast.none') }}</p>
    <template v-else-if="data">
      <p v-if="!data.listed" class="mt-3 text-[12.5px] text-faint">{{ t('channels.netcast.notListed') }}</p>
      <p v-if="!data.writable" class="mt-3 rounded-sm border border-line bg-tile px-4 py-3 text-[13px] text-muted">{{ t('channels.netcast.readOnly') }}</p>

      <!-- the preview of a change -->
      <PlanPreview v-if="plan" class="mt-4" :plan="plan" :confirm-label="t('channels.netcast.confirm')" :busy="busy" :error="error" :danger="action === 'remove'" @confirm="apply" @cancel="cancelPlan">
        <p v-for="(c, n) in plan.changes" :key="n">{{ describe(c) }}</p>
        <p class="mt-1 text-[12.5px] text-muted">{{ t('channels.netcast.backupNote') }}</p>
      </PlanPreview>

      <!-- add a source -->
      <details v-if="!plan && data.writable" ref="addBox" class="mt-4 rounded-sm border border-line bg-tile p-3" :open="addOpen" @toggle="addOpen = ($event.target as HTMLDetailsElement).open">
        <summary class="cursor-pointer text-[14px] font-semibold">{{ t('channels.netcast.add') }}</summary>
        <form class="mt-2 flex flex-col gap-2" @submit.prevent="review('add', form)">
          <fieldset class="flex flex-wrap gap-4 text-[13.5px]">
            <label class="flex items-center gap-2"><input v-model="form.kind" type="radio" value="server" /> {{ t('channels.netcast.kind.server') }}</label>
            <label class="flex items-center gap-2"><input v-model="form.kind" type="radio" value="epg" /> {{ t('channels.netcast.kind.epg') }}</label>
          </fieldset>
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ t('channels.netcast.name') }}</span>
            <input v-model="form.name" type="text" maxlength="80" class="w-full max-w-md rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
          </label>
          <label class="flex flex-col gap-0.5 text-[13px]">
            <span class="text-faint">{{ form.kind === 'epg' ? t('channels.netcast.addressEpg') : t('channels.netcast.addressServer') }}</span>
            <input v-model="form.address" type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://" class="w-full max-w-2xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
          </label>
          <template v-if="form.kind === 'server'">
            <label class="flex flex-col gap-0.5 text-[13px]">
              <span class="text-faint">{{ t('channels.netcast.mediaType') }}</span>
              <select v-model="form.mediaType" class="w-64 max-w-full rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text">
                <option v-for="m in MEDIA_TYPES" :key="m" :value="m">{{ t(`channels.netcast.media.${m}`) }}</option>
              </select>
            </label>
            <label class="flex flex-col gap-0.5 text-[13px]">
              <span class="text-faint">{{ t('channels.netcast.desc') }}</span>
              <input v-model="form.desc" type="text" maxlength="200" class="w-full max-w-md rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
            </label>
          </template>
          <div class="flex items-center gap-3">
            <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="busy || !form.name.trim() || !form.address.trim()">{{ t('channels.netcast.review') }}</button>
            <p v-if="error" class="text-[13px] text-warn" role="alert">{{ error }}</p>
          </div>
        </form>
      </details>

      <section v-for="f in data.files" :key="f.name" class="mt-4">
        <h2 class="text-[15px] font-semibold">
          {{ t(`channels.netcast.kind.${f.kind}`) }}
          <span class="ms-2 font-mono text-[12px] font-normal text-faint">{{ f.name }}</span>
        </h2>
        <p v-if="f.sources.length === 0" class="text-[13px] text-muted">{{ t('channels.netcast.empty') }}</p>
        <ul v-else class="mt-1 divide-y divide-line-soft rounded-sm border border-line bg-tile">
          <li v-for="(s, i) in f.sources" :key="s.id ?? i" class="px-3 py-2">
            <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span class="text-[14.5px] font-semibold text-text">{{ s.name }}</span>
              <span v-if="s.mediaType" class="rounded-sm border border-line px-1 text-[11.5px] text-muted">{{ s.mediaType }}</span>
              <span v-if="s.hasLogin" class="rounded-sm border border-line px-1 text-[11.5px] text-muted">{{ t('channels.iptv.login') }}</span>
              <span class="ms-auto flex gap-1">
                <button v-if="isPlaylist(f, s)" type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" :disabled="busy" @click="useList(s)">{{ t('channels.netcast.useList') }}</button>
              </span>
              <span v-if="data.writable && s.id && !plan" class="flex gap-1">
                <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" :disabled="busy" @click="startEdit(f, s)">{{ t('channels.netcast.change') }}</button>
                <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-muted hover:bg-tile-2" :disabled="busy" @click="review('remove', { kind: f.kind, name: s.name, address: '', mediaType: '', desc: '' }, f.name, s.id)">{{ t('channels.netcast.remove') }}</button>
              </span>
            </div>
            <p v-if="s.address" class="truncate text-[12.5px] text-faint">{{ s.address }}</p>
            <p v-if="s.desc" class="text-[12.5px] text-muted">{{ s.desc }}</p>
            <p v-if="listError && listFor === s.id" class="text-[13px] text-warn" role="alert">{{ listError }}</p>
            <form v-if="editing && editing.file === f.name && editing.id === s.id && !plan" class="mt-2 flex flex-col gap-2 rounded-sm border border-line-soft bg-bg p-2" @submit.prevent="review('update', edit, f.name, s.id)">
              <label class="flex flex-col gap-0.5 text-[13px]">
                <span class="text-faint">{{ t('channels.netcast.name') }}</span>
                <input v-model="edit.name" type="text" maxlength="80" class="w-full max-w-md rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
              </label>
              <label class="flex flex-col gap-0.5 text-[13px]">
                <span class="text-faint">{{ t('channels.netcast.newAddress') }}</span>
                <input v-model="edit.address" type="url" inputmode="url" autocomplete="off" spellcheck="false" placeholder="https://" class="w-full max-w-2xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
              </label>
              <div class="flex items-center gap-2">
                <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="busy">{{ t('channels.netcast.review') }}</button>
                <button type="button" class="rounded-sm px-3 py-1.5 text-[13.5px] text-muted hover:bg-tile-2" @click="editing = null">{{ t('plan.cancel') }}</button>
                <p v-if="error" class="text-[13px] text-warn" role="alert">{{ error }}</p>
              </div>
            </form>
          </li>
        </ul>
        <dl v-if="f.settings" class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-[12.5px] text-muted">
          <div v-for="(v, k) in f.settings" :key="k" class="flex gap-1">
            <dt class="font-mono text-faint">{{ k }}</dt>
            <dd>{{ v }}</dd>
          </div>
        </dl>
      </section>
      <p v-if="data.unreadable.length > 0" class="mt-4 text-[12.5px] text-warn">{{ t('channels.netcast.unreadable', { names: data.unreadable.join(', ') }) }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import ErrorText from '@/components/ErrorText.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { usePlanError } from '@/composables/usePlanError'
import { useNetcastDraft } from '@/composables/useNetcastDraft'
import { useIptvHandoff } from '@/composables/useIptvHandoff'
import type { NetcastFile, NetcastPlanBody, NetcastSource, NetcastSources, Plan, PlanChange } from '@/api/types'

const MEDIA_TYPES = ['playlist_live', 'playlist', 'playlist_vod', 'playlist_radio'] as const

interface Fields {
  kind: 'server' | 'epg'
  name: string
  address: string
  mediaType: string
  desc: string
}

const { t } = useI18n()
const { planErrorMessage } = usePlanError()
const { take } = useNetcastDraft()
const router = useRouter()
const { offer } = useIptvHandoff()
const listError = ref('')
const listFor = ref('')

const data = ref<NetcastSources | null>(null)
const loading = ref(true)
const failed = ref(false)
const busy = ref(false)
const error = ref('')
const saved = ref('')
const plan = ref<Plan | null>(null)
const action = ref<NetcastPlanBody['action']>('add')
const addOpen = ref(false)
const form = reactive<Fields>({ kind: 'server', name: '', address: '', mediaType: 'playlist_live', desc: '' })
const editing = ref<{ file: string; id: string } | null>(null)
const edit = reactive<Fields>({ kind: 'server', name: '', address: '', mediaType: '', desc: '' })

// A playlist source Netcast has downloaded, and the channel lists it keeps of
// Samsung TV Plus and Pluto, can be read like any list: the IPTV tab then picks,
// tests and puts channels into a bouquet (as entries Netcast keeps refreshing).
const listedApis = ['samsungtvplusapi', 'plutotvapi']
const isPlaylist = (f: NetcastFile, s: NetcastSource) =>
  f.kind === 'server' && !!s.id && ((s.mediaType ?? '').startsWith('playlist') || listedApis.includes(s.id))

async function useList(s: NetcastSource) {
  busy.value = true
  listError.value = ''
  listFor.value = s.id ?? ''
  try {
    offer(await api.channels.readNetcastList(s.id ?? ''))
    await router.push({ query: { tab: 'iptv' } })
  } catch (err) {
    listError.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

function describe(c: PlanChange): string {
  const b = (c.before ?? {}) as { name?: string; file?: string }
  const a = (c.after ?? {}) as { name?: string; file?: string; address?: string }
  const key = c.type.replace('netcast.source.', '')
  return t(`channels.netcast.change_${key}`, { name: a.name ?? b.name ?? '', file: a.file ?? b.file ?? '', address: a.address ?? '' })
}

async function load() {
  try {
    data.value = await api.channels.netcastSources()
    failed.value = false
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}

function startEdit(f: NetcastFile, s: NetcastSource) {
  error.value = ''
  editing.value = { file: f.name, id: s.id ?? '' }
  Object.assign(edit, { kind: f.kind, name: s.name, address: '', mediaType: '', desc: '' })
}

async function review(act: NetcastPlanBody['action'], fields: Fields, file?: string, id?: string) {
  busy.value = true
  error.value = ''
  saved.value = ''
  try {
    const body: NetcastPlanBody = { action: act, kind: fields.kind, name: fields.name.trim() }
    if (file) body.file = file
    if (id) body.id = id
    if (act !== 'remove') {
      body.address = fields.address.trim()
      if (fields.kind === 'server') {
        body.mediaType = fields.mediaType
        body.desc = fields.desc.trim()
      }
    }
    plan.value = (await api.channels.planNetcast(body)).plan
    action.value = act
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

function cancelPlan() {
  plan.value = null
  error.value = ''
}

async function apply() {
  const p = plan.value
  if (!p) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(p.id)
    plan.value = null
    editing.value = null
    addOpen.value = false
    Object.assign(form, { name: '', address: '', desc: '' })
    saved.value = t('channels.netcast.saved')
    await load()
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  const d = take()
  if (d) {
    Object.assign(form, { kind: d.kind, name: d.name, address: d.address })
    addOpen.value = true
  }
  await load()
})
</script>
