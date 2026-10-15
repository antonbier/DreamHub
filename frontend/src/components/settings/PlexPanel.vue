<!--
  Under Services → Plex (SettingsView.vue): signing in to Plex the way TV
  apps do, and choosing the libraries PlexHub shows.

  Sign-in: "Sign in with Plex" shows a short code; the user enters it at
  plex.tv/link (opened in a new tab, signed in there as usual), the panel
  notices on its own (checking every 2 s) and lists the account's servers
  with their addresses — local ones marked — plus a field for another
  address (a reverse proxy, say). "Connect" checks that the server answers
  there and saves address and token into the Plex section above.

  Connected: the server's name and the film/series libraries as
  checkboxes, saved with one button; at least one stays chosen.
-->
<template>
  <section class="rounded-sm border border-line bg-tile p-5">
    <h2 class="text-[15.5px] font-semibold">{{ t('settings.plexPanel.title') }}</h2>

    <!-- Signing in -->
    <template v-if="pin">
      <template v-if="!servers">
        <p class="mt-1 text-[13.5px] text-muted">{{ t('settings.plexPanel.enterCode') }}</p>
        <div class="mt-3 flex flex-wrap items-center gap-4">
          <span class="rounded-sm bg-field px-4 py-2 font-mono text-[26px] font-semibold tracking-[0.2em] text-text">{{ pin.code }}</span>
          <a :href="pin.linkUrl" target="_blank" rel="noopener noreferrer" class="text-[14px] text-accent hover:underline">plex.tv/link ↗</a>
        </div>
        <p class="mt-3 text-[12.5px] text-faint">{{ expired ? t('settings.plexPanel.expired') : t('settings.plexPanel.waiting') }}</p>
        <div class="mt-3 flex gap-3">
          <button v-if="expired" type="button" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg" @click="start">{{ t('settings.plexPanel.newCode') }}</button>
          <button type="button" class="text-[13px] text-muted hover:text-text" @click="reset">{{ t('settings.page.cancel') }}</button>
        </div>
      </template>
      <template v-else>
        <p class="mt-1 text-[13.5px] text-muted">{{ servers.length ? t('settings.plexPanel.chooseServer') : t('settings.plexPanel.noServers') }}</p>
        <fieldset v-for="srv in servers" :key="srv.id" class="mt-3">
          <legend class="text-[14px] font-semibold text-text">{{ srv.name }}</legend>
          <label v-for="c in srv.connections" :key="c.uri" class="mt-1.5 flex items-center gap-2 text-[13px] text-text">
            <input v-model="choice" type="radio" :value="`${srv.id}|${c.uri}`" class="accent-(--dh-accent)" />
            <span class="break-all font-mono text-[12.5px]">{{ c.uri }}</span>
            <span class="shrink-0 text-faint">{{ c.local ? t('settings.plexPanel.local') : t('settings.plexPanel.remote') }}</span>
          </label>
          <label class="mt-1.5 flex flex-wrap items-center gap-2 text-[13px] text-text">
            <input v-model="choice" type="radio" :value="`${srv.id}|custom`" class="accent-(--dh-accent)" />
            {{ t('settings.plexPanel.otherAddress') }}
            <input
              v-if="choice === `${srv.id}|custom`"
              v-model="customUrl"
              type="url"
              placeholder="https://plex.example.org"
              class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2.5 py-1.5 font-mono text-[12.5px] text-text"
            />
          </label>
        </fieldset>
        <p class="mt-3 text-[12.5px] text-faint">{{ t('settings.plexPanel.addressHint') }}</p>
        <div class="mt-3 flex items-center gap-3">
          <button
            type="button"
            class="rounded-sm bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="!choice || busy || (choice.endsWith('|custom') && !customUrl.trim())"
            @click="connect"
          >
            {{ busy ? t('settings.plexPanel.connecting') : t('settings.plexPanel.connect') }}
          </button>
          <button type="button" class="text-[13px] text-muted hover:text-text" @click="reset">{{ t('settings.page.cancel') }}</button>
        </div>
      </template>
      <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
    </template>

    <!-- Connected: libraries -->
    <template v-else-if="status?.configured">
      <p class="mt-1 text-[13.5px] text-muted">
        <span class="inline-flex items-center gap-1.5">
          <span class="h-1.5 w-1.5 rounded-full" :class="status.online ? 'bg-ok' : 'bg-warn'" />
          {{ status.online ? t('settings.plexPanel.connected', { name: status.server?.name || status.url }) : t('settings.plexPanel.offline', { url: status.url }) }}
        </span>
      </p>
      <template v-if="libraries.length">
        <p class="mt-3 text-[13.5px] text-text">{{ t('settings.plexPanel.libraries') }}</p>
        <label v-for="lib in libraries" :key="lib.id" class="mt-1.5 flex items-center gap-2 text-[13.5px] text-text">
          <input v-model="chosen" type="checkbox" :value="lib.id" class="accent-(--dh-accent)" />
          {{ lib.title }}
          <span class="text-faint">{{ t(`settings.plexPanel.kind.${lib.type}`) }}<template v-if="lib.count"> · {{ lib.count }}</template></span>
        </label>
        <div v-if="librariesChanged" class="mt-3 flex items-center gap-3">
          <button
            type="button"
            class="rounded-sm bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60"
            :disabled="chosen.length === 0 || busy"
            @click="saveLibraries"
          >
            {{ t('settings.page.save') }}
          </button>
          <span v-if="chosen.length === 0" class="text-[12.5px] text-warn">{{ t('settings.plexPanel.atLeastOne') }}</span>
        </div>
      </template>
      <button type="button" class="mt-4 text-[13px] text-accent hover:underline" @click="start">{{ t('settings.plexPanel.signInAgain') }}</button>
      <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
    </template>

    <!-- Not connected -->
    <template v-else>
      <p class="mt-1 text-[13.5px] text-muted">{{ t('settings.plexPanel.intro') }}</p>
      <button type="button" class="mt-3 rounded-sm bg-accent px-3.5 py-1.5 text-[13.5px] font-semibold text-bg disabled:opacity-60" :disabled="busy" @click="start">
        {{ t('settings.plexPanel.signIn') }}
      </button>
      <p class="mt-2 text-[12.5px] text-faint">{{ t('settings.plexPanel.manualHint') }}</p>
      <p v-if="error" class="mt-2 text-[12.5px] text-warn">{{ error }}</p>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { usePlanError } from '@/composables/usePlanError'
import type { PlexLibrary, PlexPin, PlexServerChoice, PlexStatus } from '@/api/types'

// The Plex section's values changed (connected): the page reloads them.
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { planErrorMessage } = usePlanError()

const status = ref<PlexStatus | null>(null)
const libraries = ref<PlexLibrary[]>([])
const chosen = ref<string[]>([])
const savedChoice = ref<string[]>([])
const librariesChanged = computed(() => [...chosen.value].sort().join(',') !== [...savedChoice.value].sort().join(','))
const busy = ref(false)
const error = ref('')

async function load() {
  try {
    status.value = await api.plex.status()
    if (status.value.configured) {
      const res = await api.plex.libraries()
      libraries.value = res.libraries
      chosen.value = res.libraries.filter((l) => l.selected).map((l) => l.id)
      savedChoice.value = [...chosen.value]
    }
  } catch {
    // the hub is switched off, or the server has never answered: the
    // sign-in still works
    libraries.value = []
  }
}

async function saveLibraries() {
  busy.value = true
  error.value = ''
  try {
    const res = await api.plex.selectLibraries(chosen.value)
    libraries.value = res.libraries
    savedChoice.value = [...chosen.value]
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

// --- sign-in ---
const pin = ref<PlexPin | null>(null)
const expired = ref(false)
const servers = ref<PlexServerChoice[] | null>(null)
const choice = ref('')
const customUrl = ref('')
let poll: ReturnType<typeof setInterval> | undefined

function stopPolling() {
  clearInterval(poll)
  poll = undefined
}
onBeforeUnmount(stopPolling)

async function start() {
  reset()
  busy.value = true
  try {
    pin.value = await api.plex.login.start()
    poll = setInterval(check, 2000)
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

async function check() {
  if (!pin.value) return
  try {
    const res = await api.plex.login.check(pin.value.id)
    if (res.state === 'expired') {
      expired.value = true
      stopPolling()
    } else if (res.state === 'linked') {
      stopPolling()
      servers.value = res.servers ?? []
      // Preselect the first local address of the first server.
      const first = servers.value[0]
      const local = first?.connections.find((c) => c.local) ?? first?.connections[0]
      if (first && local) choice.value = `${first.id}|${local.uri}`
    }
  } catch {
    // plex.tv hiccup: the next check tries again
  }
}

async function connect() {
  if (!pin.value) return
  const [serverId, uri] = choice.value.split('|')
  const url = uri === 'custom' ? customUrl.value.trim() : uri
  busy.value = true
  error.value = ''
  try {
    await api.plex.login.use(pin.value.id, serverId, url)
    reset()
    emit('changed')
    await load()
  } catch (err) {
    error.value = planErrorMessage(err)
  } finally {
    busy.value = false
  }
}

function reset() {
  stopPolling()
  pin.value = null
  servers.value = null
  expired.value = false
  choice.value = ''
  customUrl.value = ''
  error.value = ''
}

onMounted(load)
</script>
