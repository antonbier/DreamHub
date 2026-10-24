<!--
  Guide sources for the chosen channels (docs/NETCAST_PLAN.md, N5): pick an
  XMLTV file — from the suggested list, an address, or a public GitHub
  repository — and see how many of the channels it knows. Only the file's
  channel list is read, by DreamHub, never by the browser; nothing is
  changed on the box. Putting a source into Netcast comes with the box
  settings (N3).
-->
<template>
  <details class="rounded-sm border border-line bg-tile p-3" @toggle="onToggle">
    <summary class="cursor-pointer text-[14px] font-semibold">{{ t('channels.iptv.guideSources.title') }}</summary>
    <p class="mt-1 text-[12.5px] text-muted">{{ t('channels.iptv.guideSources.explain') }}</p>

    <div v-if="opened" class="mt-3 flex flex-col gap-3">
      <label v-if="curated.length > 0" class="flex flex-col gap-0.5 text-[13px]">
        <span class="text-faint">{{ t('channels.iptv.guideSources.curated') }}</span>
        <select v-model="curatedId" class="max-w-full rounded-sm border border-line bg-field px-2 py-1.5 text-[13.5px] text-text" @change="useCurated">
          <option value="">—</option>
          <option v-for="c in curated" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </label>

      <label class="flex flex-col gap-0.5 text-[13px]">
        <span class="text-faint">{{ t('channels.iptv.guideSources.own') }}</span>
        <input v-model="url" type="url" inputmode="url" autocomplete="off" spellcheck="false" class="w-full max-w-xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" @input="curatedId = ''; result = null" />
      </label>

      <form class="flex flex-col gap-1" @submit.prevent="listRepo(repo)">
        <label class="flex flex-col gap-0.5 text-[13px]">
          <span class="text-faint">{{ t('channels.iptv.guideSources.repo') }} · {{ t('channels.iptv.guideSources.repoHint') }}</span>
          <span class="flex flex-wrap items-center gap-2">
            <input v-model="repo" type="text" autocomplete="off" spellcheck="false" class="w-full max-w-xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
            <button type="submit" class="rounded-sm border border-line px-3 py-1.5 text-[13.5px] text-text hover:bg-tile-2 disabled:opacity-50" :disabled="listing || repo.trim() === ''">
              {{ listing ? t('channels.iptv.guideSources.listing') : t('channels.iptv.guideSources.list') }}
            </button>
          </span>
        </label>
        <p v-if="repoError" class="text-[13px] text-warn" role="alert"><ErrorText :error="repoError" /></p>
      </form>

      <div v-if="listed">
        <h3 class="text-[13px] font-semibold text-text">{{ t('channels.iptv.guideSources.files') }} · {{ listed.repo }}{{ listed.dir ? '/' + listed.dir : '' }}</h3>
        <p v-if="listed.files.length === 0" class="text-[13px] text-muted">{{ t('channels.iptv.guideSources.noFiles') }}</p>
        <ul v-else class="mt-1 divide-y divide-line-soft rounded-sm border border-line bg-bg">
          <li v-for="f in listed.files" :key="f.path" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5">
            <span class="min-w-0 grow basis-40 truncate text-[14px] text-text">{{ f.name }}</span>
            <span class="text-[12px] text-faint">{{ f.kind === 'playlist' ? t('channels.iptv.guideSources.playlistFile') : size(f.size) }}</span>
            <button v-if="f.kind === 'guide'" type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="useFile(f.url)">
              {{ t('channels.iptv.guideSources.useGuide') }}
            </button>
          </li>
        </ul>
        <div v-if="listed.dirs.length > 0" class="mt-2">
          <h3 class="text-[13px] text-faint">{{ t('channels.iptv.guideSources.folders') }}</h3>
          <ul class="flex flex-wrap gap-2">
            <li v-for="d in listed.dirs" :key="d">
              <button type="button" class="rounded-sm border border-line px-2 py-1 text-[13px] text-text hover:bg-tile-2" :disabled="listing" :title="t('channels.iptv.guideSources.openFolder')" @click="openDir(d)">{{ d }}</button>
            </li>
          </ul>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-3 text-[13px]">
        <button type="button" class="rounded-sm bg-accent px-3 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-50" :disabled="checking || url.trim() === ''" @click="check">
          {{ checking ? t('channels.iptv.guideSources.checking') : t('channels.iptv.guideSources.check') }}
        </button>
        <span class="text-faint">{{ ids.length > 0 ? t('channels.iptv.guideSources.scope', { n: number(ids.length) }) : t('channels.iptv.guideSources.scopeAll', { n: number(total) }) }}</span>
      </div>
      <p v-if="checkError" class="text-[13px] text-warn" role="alert"><ErrorText :error="checkError" /></p>

      <div v-if="result" role="status">
        <p class="text-[14px] font-semibold text-text">{{ t('channels.iptv.guideSources.result', { known: number(result.known), total: number(result.total) }) }}</p>
        <p class="text-[12.5px] text-faint">
          {{ t('channels.iptv.guideSources.sourceHas', { n: number(result.sourceChannels) }) }}
          <template v-if="result.truncated">{{ t('channels.iptv.guideSources.truncated', { n: number(result.sourceChannels) }) }}</template>
        </p>
        <details v-if="result.missingCount > 0" class="mt-1">
          <summary class="cursor-pointer text-[13px] text-muted">{{ t('channels.iptv.guideSources.missing', { n: number(result.missingCount) }) }}</summary>
          <ul class="mt-1 columns-2 text-[13px] text-text">
            <li v-for="m in result.missing" :key="m.id" class="truncate">{{ m.name }}</li>
          </ul>
          <p v-if="result.missingCount > result.missing.length" class="text-[12.5px] text-faint">{{ t('channels.iptv.guideSources.missingMore', { n: number(result.missingCount - result.missing.length) }) }}</p>
        </details>
      </div>
    </div>
  </details>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { GuideCoverage, GuideSource, RepoListing } from '@/api/types'
import ErrorText from '@/components/ErrorText.vue'

const props = defineProps<{ token: string; ids: number[]; total: number }>()
const { t, n: formatNumber } = useI18n()
const number = (v: number) => formatNumber(v)

const opened = ref(false)
const curated = ref<GuideSource[]>([])
const curatedId = ref('')
const url = ref('')
const repo = ref('')
const listed = ref<RepoListing | null>(null)
const listing = ref(false)
const repoError = ref<unknown>(null)
const checking = ref(false)
const checkError = ref<unknown>(null)
const result = ref<GuideCoverage | null>(null)

function size(n: number): string {
  return n >= 1 << 20 ? `${number(Math.round((n / (1 << 20)) * 10) / 10)} MB` : `${number(Math.max(1, Math.round(n / 1024)))} KB`
}

async function onToggle(e: Event) {
  opened.value = (e.target as HTMLDetailsElement).open
  if (opened.value && curated.value.length === 0) {
    try {
      curated.value = (await api.channels.guideSources()).sources
    } catch {
      curated.value = []
    }
  }
}

function useCurated() {
  const c = curated.value.find((x) => x.id === curatedId.value)
  result.value = null
  url.value = c ? c.url : ''
}

function useFile(address: string) {
  curatedId.value = ''
  url.value = address
  result.value = null
}

async function listRepo(address: string) {
  listing.value = true
  repoError.value = null
  try {
    listed.value = await api.channels.listGithub(address.trim())
  } catch (e) {
    listed.value = null
    repoError.value = e
  } finally {
    listing.value = false
  }
}

function openDir(dir: string) {
  if (!listed.value) return
  void listRepo(`https://github.com/${listed.value.repo}/tree/${listed.value.ref || 'HEAD'}/${dir}`)
}

async function check() {
  checking.value = true
  checkError.value = null
  result.value = null
  try {
    result.value = await api.channels.guideCoverage(props.token, props.ids, url.value.trim())
  } catch (e) {
    checkError.value = e
  } finally {
    checking.value = false
  }
}
</script>
