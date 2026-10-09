<!--
  The profile's own PIN (docs/archive/PROFILES_PLAN.md): every profile can set,
  change or remove it — except an admin's once other profiles exist, which
  the server refuses with profile_admin_pin_required.
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('profiles.myPin.title') }}</h2>
    <p class="mb-4 text-[13px] text-muted">
      {{ me?.hasPin ? t('profiles.myPin.hasPin') : t('profiles.myPin.noPin') }}
    </p>
    <form class="flex flex-wrap items-end gap-2" @submit.prevent="save(pin)">
      <label class="block">
        <span class="mb-1 block text-[13px] text-muted">{{ me?.hasPin ? t('profiles.myPin.newPin') : t('profiles.myPin.pin') }}</span>
        <input
          v-model="pin"
          type="password"
          inputmode="numeric"
          autocomplete="new-password"
          maxlength="8"
          class="w-[160px] rounded border border-line bg-field px-2.5 py-1.5 font-mono text-[14px] text-text"
        />
      </label>
      <button type="submit" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || !/^\d{4,8}$/.test(pin)">
        {{ t('profiles.myPin.save') }}
      </button>
      <button v-if="me?.hasPin" type="button" class="rounded px-3 py-1.5 text-[13px] text-muted hover:bg-tile-2 hover:text-text" :disabled="busy" @click="save('')">
        {{ t('profiles.myPin.remove') }}
      </button>
    </form>
    <p class="mt-2 text-[12.5px] text-faint">{{ t('profiles.pinFormat') }}</p>
    <p v-if="message" class="mt-2 text-[13px]" :class="failed ? 'text-warn' : 'text-ok'" role="status">{{ message }}</p>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { useAuth } from '@/composables/useAuth'
import { profileErrorKey } from './profileErrors'

const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const { me, check } = useAuth()
const pin = ref('')
const busy = ref(false)
const message = ref('')
const failed = ref(false)

async function save(value: string) {
  busy.value = true
  message.value = ''
  try {
    await api.profiles.setMyPin(value)
    pin.value = ''
    failed.value = false
    message.value = value ? t('profiles.myPin.saved') : t('profiles.myPin.removed')
    // A new PIN ends this profile's sessions, this device's too: the
    // picker comes next if the household uses one.
    await check()
    emit('changed')
  } catch (err) {
    failed.value = true
    message.value = t(profileErrorKey(err))
  } finally {
    busy.value = false
  }
}
</script>
