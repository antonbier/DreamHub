<!--
  One box's fields, for adding or editing (BoxesPanel.vue). The first box's
  address and login are set up under Services → WebControl, so only name
  and colour are offered for it. "Test" tries the address and login and
  names the device that answered; nothing is saved by it.
-->
<template>
  <form class="rounded-sm border border-line bg-field p-4" @submit.prevent="submit">
    <label class="mb-3 block">
      <span class="mb-1 block text-[13px] text-muted">{{ t('boxes.form.name') }}</span>
      <input v-model="name" type="text" maxlength="30" class="w-full max-w-[280px] rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" />
    </label>

    <fieldset class="mb-3">
      <legend class="mb-1 text-[13px] text-muted">{{ t('boxes.form.color') }}</legend>
      <div class="flex flex-wrap gap-2">
        <label v-for="c in COLORS" :key="c" class="cursor-pointer">
          <input v-model="color" type="radio" name="box-color" :value="c" class="peer sr-only" />
          <span
            class="block h-7 w-7 rounded-full ring-offset-2 ring-offset-field peer-checked:ring-2 peer-checked:ring-text peer-focus-visible:ring-2 peer-focus-visible:ring-accent"
            :style="{ background: `var(--dh-profile-${c})` }"
          />
          <span class="sr-only">{{ t(`profiles.color.${c}`) }}</span>
        </label>
      </div>
    </fieldset>

    <template v-if="!box?.full">
      <label class="mb-3 block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('boxes.form.url') }}</span>
        <input
          v-model="url"
          type="text"
          inputmode="url"
          autocapitalize="off"
          spellcheck="false"
          placeholder="192.168.1.20"
          class="w-full max-w-[280px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[14px] text-text"
        />
        <span class="mt-1 block text-[12.5px] text-faint">{{ t('boxes.form.urlHint') }}</span>
      </label>
      <div class="mb-3 flex flex-wrap gap-3">
        <label class="block">
          <span class="mb-1 block text-[13px] text-muted">{{ t('boxes.form.username') }}</span>
          <input v-model="username" type="text" autocomplete="off" class="w-[180px] rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" />
        </label>
        <label class="block">
          <span class="mb-1 block text-[13px] text-muted">{{ box?.hasPassword ? t('boxes.form.newPassword') : t('boxes.form.password') }}</span>
          <input v-model="password" type="password" autocomplete="new-password" class="w-[180px] rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" />
        </label>
      </div>
      <p class="mb-3 text-[12.5px] text-faint">{{ t('boxes.form.loginHint') }}</p>

      <div class="mb-3 flex flex-wrap items-center gap-2">
        <button type="button" class="rounded-sm border border-line px-3 py-1.5 text-[13px] text-text hover:bg-tile disabled:opacity-60" :disabled="testing || busy || !url.trim() || (box?.hasPassword && !password)" @click="runTest">
          {{ testing ? t('boxes.form.testing') : t('boxes.form.test') }}
        </button>
        <span v-if="testResult" class="text-[13px]" :class="testResult.ok ? 'text-ok' : 'text-warn'" role="status">{{ testResult.text }}</span>
      </div>
    </template>

    <p v-if="error" class="mb-3 text-[13px] text-warn" role="alert">{{ error }}</p>
    <div class="flex flex-wrap items-center gap-2">
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || !valid">
        {{ busy ? t('settings.page.saving') : box ? t('boxes.form.save') : t('boxes.form.addBox') }}
      </button>
      <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-muted hover:bg-tile hover:text-text" @click="emit('cancel')">
        {{ t('settings.page.cancel') }}
      </button>
      <button v-if="box && !box.full" type="button" class="ms-auto rounded-sm px-3 py-1.5 text-[13px] text-warn hover:bg-tile" :disabled="busy" @click="emit('remove')">
        {{ t('boxes.form.remove') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import { boxErrorKey } from './boxErrors'
import type { BoxInfo, BoxInput } from '@/api/types'

const props = defineProps<{ box?: BoxInfo; busy: boolean; error: string }>()
const emit = defineEmits<{ save: [input: BoxInput]; remove: []; cancel: [] }>()
const { t } = useI18n()

// The same palette as the profiles (backend boxes.Colors).
const COLORS = ['blue', 'green', 'amber', 'red', 'violet', 'teal', 'pink', 'slate'] as const

const name = ref(props.box?.name ?? '')
const color = ref(props.box?.color || COLORS[0])
const url = ref(props.box?.url ?? '')
const username = ref(props.box?.username ?? '')
const password = ref('')
const testing = ref(false)
const testResult = ref<{ ok: boolean; text: string } | null>(null)

const valid = computed(() => name.value.trim() !== '' && (props.box?.full || url.value.trim() !== ''))

function connection(): BoxInput {
  const input: BoxInput = { url: url.value.trim(), username: username.value.trim() }
  if (password.value) input.password = password.value
  return input
}

async function runTest() {
  testing.value = true
  testResult.value = null
  try {
    const res = await api.boxes.test(connection())
    testResult.value = { ok: true, text: t('boxes.form.testOk', { device: res.deviceName || url.value }) }
  } catch (err) {
    testResult.value = { ok: false, text: t(boxErrorKey(err)) }
  } finally {
    testing.value = false
  }
}

function submit() {
  const input: BoxInput = { name: name.value.trim(), color: color.value }
  if (!props.box?.full) Object.assign(input, connection())
  emit('save', input)
}
</script>
