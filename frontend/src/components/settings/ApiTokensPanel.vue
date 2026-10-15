<!--
  API tokens (PRINCIPLES.md: "a separate, user-generatable API token...
  not tied to the Basic Auth password"). Sits right after the
  Authentication section in SettingsView.vue — it's not a schema field
  (tokens are a list of their own resources, not a single setting), so it's
  the one hand-built exception to the otherwise fully generic settings page.
-->
<template>
  <section class="rounded-sm border border-line bg-tile p-5">
    <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('settings.tokens.title') }}</h2>
    <p class="mb-4 text-[13px] text-muted">{{ t('settings.tokens.subtitle') }}</p>

    <div v-if="created" class="mb-4 rounded-sm border border-accent bg-field p-3.5">
      <p class="mb-2 text-[13px] text-text">{{ t('settings.tokens.createdOnce') }}</p>
      <div class="flex items-center gap-2">
        <code class="flex-1 overflow-x-auto whitespace-nowrap rounded-sm bg-tile px-2 py-1.5 font-mono text-[12.5px] text-text">{{ created.token }}</code>
        <button type="button" class="shrink-0 rounded-sm px-2.5 py-1.5 text-[12.5px] text-accent hover:bg-tile-2" @click="copy">
          {{ copied ? t('settings.tokens.copied') : t('settings.tokens.copy') }}
        </button>
      </div>
      <button type="button" class="mt-2 text-[12.5px] text-muted hover:text-text" @click="created = null">
        {{ t('settings.page.cancel') }}
      </button>
    </div>

    <ul v-if="tokens.length > 0" class="mb-4 flex flex-col">
      <li v-for="token in tokens" :key="token.id" class="flex items-center gap-3 border-b border-line-soft py-2 last:border-none">
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14px] text-text">{{ token.name }}</span>
          <span class="block text-[12px] text-faint">{{ t('settings.tokens.createdAt', { date: formatDate(token.createdAt) }) }}</span>
        </span>
        <button
          type="button"
          class="shrink-0 text-[12.5px]"
          :class="confirmingId === token.id ? 'text-warn' : 'text-accent hover:underline'"
          @click="revoke(token.id)"
        >
          {{ confirmingId === token.id ? t('settings.tokens.confirmRevoke') : t('settings.tokens.revoke') }}
        </button>
      </li>
    </ul>
    <p v-else-if="!loading" class="mb-4 text-[13px] text-faint">{{ t('settings.tokens.empty') }}</p>

    <form class="flex items-center gap-2" @submit.prevent="create">
      <input
        v-model="newName"
        type="text"
        :placeholder="t('settings.tokens.namePlaceholder')"
        class="max-w-[240px] flex-1 rounded-sm border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
      />
      <button
        type="submit"
        class="shrink-0 rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60"
        :disabled="!newName.trim() || creating"
      >
        {{ creating ? t('settings.page.saving') : t('settings.tokens.create') }}
      </button>
    </form>
    <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
  </section>
</template>

<script setup lang="ts">
import { useFriendlyError } from '@/composables/useFriendlyError'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useDisplayTimeZone } from '@/composables/useDisplayTimeZone'
import type { ApiToken, CreatedApiToken } from '@/api/types'

const { t, locale } = useI18n()
const { friendly } = useFriendlyError()
const { timeZone } = useDisplayTimeZone()
const tokens = ref<ApiToken[]>([])
const loading = ref(true)
const newName = ref('')
const creating = ref(false)
const error = ref('')
const created = ref<CreatedApiToken | null>(null)
const copied = ref(false)
const confirmingId = ref<number | null>(null)

function formatDate(iso: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeZone: timeZone.value }).format(new Date(iso))
}

async function load() {
  loading.value = true
  try {
    tokens.value = (await api.tokens.list()).tokens
  } catch {
    // The settings page already surfaces backend-unreachable state
    // elsewhere; an empty list here is a reasonable fallback.
  } finally {
    loading.value = false
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name) return
  creating.value = true
  error.value = ''
  try {
    created.value = await api.tokens.create(name)
    copied.value = false
    newName.value = ''
    await load()
  } catch (err) {
    error.value = friendly(err).text
  } finally {
    creating.value = false
  }
}

async function copy() {
  if (!created.value) return
  try {
    await navigator.clipboard.writeText(created.value.token)
    copied.value = true
  } catch {
    // Clipboard access can be blocked; the token is still selectable text.
  }
}

async function revoke(id: number) {
  if (confirmingId.value !== id) {
    confirmingId.value = id
    return
  }
  confirmingId.value = null
  try {
    await api.tokens.revoke(id)
    await load()
  } catch (err) {
    error.value = friendly(err).text
  }
}

onMounted(load)
</script>
