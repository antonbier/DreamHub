<!--
  Choose the logo of one channel (docs/CHANNELHUB_PLAN.md, #51): the
  picture an installed set has for it, one of your own, or one from the
  public TV-logo collection. It is DreamHub's own choice — the TV keeps
  showing what the box has — and "automatic" removes it again. Three ways,
  all visible at once; no mode to switch.
-->
<template>
  <div class="rounded-sm border border-line bg-tile p-3">
    <div class="flex items-center gap-3">
      <ChannelLogo source="channels" :service-reference="channel.ref" :service-name="channel.name" :height="32" />
      <span class="min-w-0 flex-1 truncate text-[14px] font-semibold text-text">{{ channel.name }}</span>
      <button v-if="custom" type="button" class="shrink-0 text-[12.5px] text-accent hover:underline" :disabled="busy" @click="reset">{{ t('channels.logo.reset') }}</button>
      <button type="button" class="shrink-0 text-[12.5px] text-muted hover:text-text" @click="$emit('close')">{{ t('channels.logo.close') }}</button>
    </div>

    <h3 class="mt-3 text-[12.5px] font-semibold text-muted">{{ t('channels.logo.fromSets') }}</h3>
    <p v-if="loadingSets" class="mt-1 text-[13px] text-faint">{{ t('common.loading') }}</p>
    <p v-else-if="sets.length === 0" class="mt-1 text-[13px] text-faint">{{ t('channels.logo.noSets') }}</p>
    <div v-else class="mt-2 flex flex-wrap gap-2">
      <button
        v-for="s in sets"
        :key="s"
        type="button"
        class="flex flex-col items-center gap-1 rounded-sm border border-line bg-bg p-2 hover:bg-field disabled:opacity-60"
        :disabled="busy"
        :title="s"
        @click="chooseSet(s)"
      >
        <LogoThumb :load="() => loadSetPicon(s)" />
        <span class="max-w-[110px] truncate text-[11px] text-faint">{{ s }}</span>
      </button>
    </div>

    <h3 class="mt-4 text-[12.5px] font-semibold text-muted">{{ t('channels.logo.upload') }}</h3>
    <div class="mt-1 flex flex-wrap items-center gap-2">
      <input ref="fileInput" type="file" accept="image/png,image/jpeg,image/gif" class="hidden" @change="onFile" />
      <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60" :disabled="busy" @click="fileInput?.click()">
        {{ t('channels.logo.chooseFile') }}
      </button>
      <span class="text-[12px] text-faint">{{ t('channels.logo.uploadHint') }}</span>
    </div>

    <template v-if="onlineAvailable">
      <h3 class="mt-4 text-[12.5px] font-semibold text-muted">{{ t('channels.logo.online') }}</h3>
      <form class="mt-1 flex gap-2" @submit.prevent="search">
        <input v-model="query" type="search" class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2.5 py-1.5 text-[13px] text-text" :aria-label="t('channels.logo.online')" />
        <button type="submit" class="shrink-0 rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-field disabled:opacity-60" :disabled="searching || !query.trim()">
          {{ t('channels.logo.search') }}
        </button>
      </form>
      <p v-if="searching" class="mt-2 text-[13px] text-faint">{{ t('common.loading') }}</p>
      <p v-else-if="searchError" class="mt-2 text-[12.5px] text-warn">{{ searchError }}</p>
      <p v-else-if="searched && results.length === 0" class="mt-2 text-[13px] text-faint">{{ t('channels.logo.noResults') }}</p>
      <div v-else-if="results.length > 0" class="mt-2 flex flex-wrap gap-2">
        <button
          v-for="r in results"
          :key="r.path"
          type="button"
          class="flex flex-col items-center gap-1 rounded-sm border border-line bg-bg p-2 hover:bg-field disabled:opacity-60"
          :disabled="busy"
          :title="r.path"
          @click="chooseOnline(r)"
        >
          <LogoThumb :load="() => loadPreview(r.path)" />
          <span class="max-w-[110px] truncate text-[11px] text-faint">{{ r.name }} · {{ r.country }}</span>
        </button>
      </div>
      <p v-if="searched" class="mt-2 text-[11.5px] text-faint">{{ t('channels.logo.source') }}</p>
    </template>

    <p v-if="error" class="mt-3 text-[12.5px] text-warn" role="alert">{{ error }}</p>
    <p v-if="saved" class="mt-3 text-[12.5px] text-ok" role="status">{{ t('channels.logo.saved') }}</p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api, ApiError } from '@/api/client'
import ChannelLogo, { invalidateLogos } from '@/components/guide/ChannelLogo.vue'
import LogoThumb from '@/components/channels/LogoThumb.vue'
import { useFriendlyError } from '@/composables/useFriendlyError'
import type { LogoResult } from '@/api/types'

const props = defineProps<{ channel: { ref: string; name: string } }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const { t } = useI18n()
const { friendly } = useFriendlyError()

const sets = ref<string[]>([])
const custom = ref(false)
const loadingSets = ref(true)
const busy = ref(false)
const error = ref('')
const saved = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const query = ref(props.channel.name)
const results = ref<LogoResult[]>([])
const searching = ref(false)
const searched = ref(false)
const searchError = ref('')
const onlineAvailable = ref(true)

async function loadSets() {
  try {
    const res = await api.channels.logoSets(props.channel.ref, props.channel.name)
    sets.value = res.sets
    custom.value = res.custom
  } catch {
    sets.value = []
  } finally {
    loadingSets.value = false
  }
}

// Every change goes through here: save, then show it everywhere.
async function save(action: () => Promise<unknown>) {
  busy.value = true
  error.value = ''
  saved.value = false
  try {
    await action()
    invalidateLogos()
    saved.value = true
    custom.value = true
    emit('changed')
  } catch (err) {
    error.value = err instanceof ApiError && ['logo_invalid', 'logo_too_large'].includes(err.code) ? err.message : friendly(err).text
  } finally {
    busy.value = false
  }
}

// Thumbnails fetch their picture through these (not `api` straight from the
// template, which vue-tsc 3 mistakes for a ref).
const loadSetPicon = (set: string) => api.channels.piconInSet(props.channel.ref, props.channel.name, set)
const loadPreview = (path: string) => api.channels.logoPreview(path)
const chooseSet = (set: string) => save(() => api.channels.chooseLogo(props.channel.ref, props.channel.name, set))
const chooseOnline = (r: LogoResult) => save(() => api.channels.fetchLogo(props.channel.ref, r.path))

function onFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) void save(() => api.channels.uploadLogo(props.channel.ref, file))
}

async function reset() {
  busy.value = true
  error.value = ''
  try {
    await api.channels.resetLogo(props.channel.ref)
    invalidateLogos()
    custom.value = false
    saved.value = false
    emit('changed')
  } catch (err) {
    error.value = friendly(err).text
  } finally {
    busy.value = false
  }
}

async function search() {
  searching.value = true
  searchError.value = ''
  try {
    results.value = (await api.channels.searchLogos(query.value.trim())).results
    searched.value = true
  } catch (err) {
    if (err instanceof ApiError && err.code === 'logos_unavailable') onlineAvailable.value = false
    else searchError.value = err instanceof ApiError && err.code === 'logos_unreachable' ? t('channels.logo.unreachable') : friendly(err).text
  } finally {
    searching.value = false
  }
}

onMounted(loadSets)
</script>
