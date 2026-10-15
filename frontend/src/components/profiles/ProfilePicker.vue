<!--
  "Who's watching?" (docs/archive/PROFILES_PLAN.md, P2): shown instead of the shell
  when the door is open but this device hasn't picked a profile (App.vue).
  A profile with a PIN asks for it; the choice is remembered on the device.
-->
<template>
  <div class="flex min-h-screen w-screen items-center justify-center bg-bg px-4 py-8 text-text">
    <div class="w-full max-w-[560px]">
      <h1 class="mb-6 text-center text-[24px] font-bold">{{ t('profiles.picker.title') }}</h1>

      <p v-if="loading" class="text-center text-[14px] text-muted">{{ t('common.loading') }}</p>
      <div v-else-if="loadFailed" class="text-center">
        <p class="mb-3 text-[14px] text-muted">{{ t('profiles.picker.loadFailed') }}</p>
        <button type="button" class="text-[13.5px] text-accent hover:underline" @click="load">{{ t('common.retry') }}</button>
      </div>

      <form v-else-if="chosen" class="mx-auto flex max-w-[300px] flex-col items-center" @submit.prevent="submitPin">
        <ProfileAvatar :name="chosen.name" :color="chosen.color" :size="72" />
        <p class="mb-4 mt-2 text-[16px] font-semibold">{{ chosen.name }}</p>
        <label class="mb-3 block w-full">
          <span class="mb-1 block text-[13px] text-muted">{{ t('profiles.picker.pin') }}</span>
          <input
            ref="pinInput"
            v-model="pin"
            type="password"
            inputmode="numeric"
            autocomplete="off"
            maxlength="8"
            class="w-full rounded-sm border border-line bg-field px-2.5 py-2 text-center font-mono text-[20px] tracking-[0.3em] text-text"
          />
        </label>
        <p v-if="error" class="mb-3 text-[13px] text-warn" role="alert">{{ error }}</p>
        <button
          type="submit"
          class="w-full rounded-sm bg-accent py-2 text-[14px] font-semibold text-bg disabled:opacity-60"
          :disabled="busy || pin.length < 4"
        >
          {{ busy ? t('profiles.picker.checking') : t('profiles.picker.open') }}
        </button>
        <button type="button" class="mt-3 text-[13px] text-muted hover:text-text" @click="back">
          {{ t('profiles.picker.back') }}
        </button>
      </form>

      <template v-else>
        <ul class="flex flex-wrap justify-center gap-4">
          <li v-for="p in profiles" :key="p.id">
            <button
              type="button"
              class="flex w-[120px] flex-col items-center gap-2 rounded-lg p-3 hover:bg-tile focus-visible:bg-tile"
              :disabled="busy"
              @click="choose(p)"
            >
              <ProfileAvatar :name="p.name" :color="p.color" :size="72" />
              <span class="max-w-full truncate text-[14.5px] font-semibold">{{ p.name }}</span>
              <span v-if="p.hasPin" class="flex items-center gap-1 text-[12px] text-muted"><Icon name="lock" :size="13" />{{ t('profiles.picker.hasPin') }}</span>
            </button>
          </li>
        </ul>
        <p v-if="error" class="mt-4 text-center text-[13px] text-warn" role="alert">{{ error }}</p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ProfileAvatar from './ProfileAvatar.vue'
import Icon from '@/components/Icon.vue'
import { api, ApiError } from '@/api/client'
import { authState, setProfileToken } from '@/api/authState'
import type { Profile } from '@/api/types'

const emit = defineEmits<{ chosen: [] }>()
const { t } = useI18n()

const profiles = ref<Profile[]>([])
const loading = ref(true)
const loadFailed = ref(false)
const chosen = ref<Profile | null>(null)
const pin = ref('')
const busy = ref(false)
const error = ref('')
const pinInput = ref<HTMLInputElement | null>(null)

async function load() {
  loading.value = true
  loadFailed.value = false
  try {
    profiles.value = (await api.profiles.list()).profiles
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function choose(p: Profile) {
  error.value = ''
  if (p.hasPin) {
    chosen.value = p
    pin.value = ''
    await nextTick()
    pinInput.value?.focus()
    return
  }
  await open(p, undefined)
}

function submitPin() {
  if (chosen.value) void open(chosen.value, pin.value)
}

function back() {
  chosen.value = null
  error.value = ''
}

async function open(p: Profile, withPin: string | undefined) {
  busy.value = true
  error.value = ''
  try {
    const res = await api.profiles.startSession(p.id, withPin)
    setProfileToken(res.token)
    authState.profileRequired = false
    emit('chosen')
  } catch (err) {
    pin.value = ''
    if (err instanceof ApiError && err.code === 'pin_wrong') error.value = t('profiles.picker.wrongPin')
    else if (err instanceof ApiError && err.code === 'pin_locked') error.value = t('profiles.picker.locked')
    else error.value = t('profiles.picker.failed')
    await nextTick()
    pinInput.value?.focus()
  } finally {
    busy.value = false
  }
}
</script>
