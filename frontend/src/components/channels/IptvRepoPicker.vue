<!--
  A playlist from a public GitHub repository (docs/NETCAST_PLAN.md, N5): paste
  a repository or folder address, see its .m3u files, use one — its raw
  address goes into the address field of the reading form. The backend lists
  the files; nothing is read until the list itself is read.
-->
<template>
  <details class="w-full rounded-sm border border-line bg-tile p-3">
    <summary class="cursor-pointer text-[13.5px] font-semibold">{{ t('channels.iptv.repo.title') }}</summary>
    <form class="mt-2 flex flex-wrap items-end gap-2" @submit.prevent="list(address)">
      <label class="flex min-w-0 grow flex-col gap-0.5 text-[13px]">
        <span class="text-faint">{{ t('channels.iptv.guideSources.repo') }} · {{ t('channels.iptv.guideSources.repoHint') }}</span>
        <input v-model="address" type="text" autocomplete="off" spellcheck="false" class="w-full max-w-xl rounded-sm border border-line bg-field px-2 py-1.5 text-[14px] text-text" />
      </label>
      <button type="submit" class="rounded-sm border border-line px-3 py-1.5 text-[13.5px] text-text hover:bg-tile-2 disabled:opacity-50" :disabled="busy || address.trim() === ''">
        {{ busy ? t('channels.iptv.guideSources.listing') : t('channels.iptv.guideSources.list') }}
      </button>
    </form>
    <p v-if="error" class="mt-1 text-[13px] text-warn" role="alert"><ErrorText :error="error" /></p>
    <div v-if="listed" class="mt-2">
      <h3 class="text-[13px] font-semibold text-text">{{ t('channels.iptv.guideSources.files') }} · {{ listed.repo }}{{ listed.dir ? '/' + listed.dir : '' }}</h3>
      <p v-if="playlists.length === 0" class="text-[13px] text-muted">{{ t('channels.iptv.repo.noFiles') }}</p>
      <ul v-else class="mt-1 divide-y divide-line-soft rounded-sm border border-line bg-bg">
        <li v-for="f in playlists" :key="f.path" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5">
          <span class="min-w-0 grow basis-40 truncate text-[14px] text-text">{{ f.name }}</span>
          <button type="button" class="rounded-sm px-2 py-1 text-[13px] text-accent hover:bg-tile-2" @click="emit('pick', f.url)">{{ t('channels.iptv.repo.use') }}</button>
        </li>
      </ul>
      <ul v-if="listed.dirs.length > 0" class="mt-2 flex flex-wrap gap-2">
        <li v-for="d in listed.dirs" :key="d">
          <button type="button" class="rounded-sm border border-line px-2 py-1 text-[13px] text-text hover:bg-tile-2" :disabled="busy" @click="open(d)">{{ d }}</button>
        </li>
      </ul>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { RepoListing } from '@/api/types'
import ErrorText from '@/components/ErrorText.vue'

const emit = defineEmits<{ pick: [url: string] }>()
const { t } = useI18n()

const address = ref('')
const listed = ref<RepoListing | null>(null)
const busy = ref(false)
const error = ref<unknown>(null)
const playlists = computed(() => listed.value?.files.filter((f) => f.kind === 'playlist') ?? [])

async function list(a: string) {
  busy.value = true
  error.value = null
  try {
    listed.value = await api.channels.listGithub(a.trim())
  } catch (e) {
    listed.value = null
    error.value = e
  } finally {
    busy.value = false
  }
}

function open(dir: string) {
  if (listed.value) void list(`https://github.com/${listed.value.repo}/tree/${listed.value.ref || 'HEAD'}/${dir}`)
}
</script>
