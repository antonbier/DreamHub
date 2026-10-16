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

    <fieldset class="mb-3">
      <legend class="mb-1 text-[13px] text-muted">{{ t('boxes.form.wake.legend') }}</legend>
      <select v-model="wakeMethod" class="rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" :aria-label="t('boxes.form.wake.legend')">
        <option value="">{{ t('boxes.form.wake.none') }}</option>
        <option value="wol">{{ t('boxes.form.wake.wol') }}</option>
        <option value="extension">{{ t('boxes.form.wake.extension') }}</option>
      </select>
      <label v-if="wakeMethod === 'wol'" class="mt-2 block">
        <span class="mb-1 block text-[13px] text-muted">{{ t('boxes.form.wake.mac') }}</span>
        <input
          v-model="wakeMac"
          type="text"
          autocapitalize="off"
          spellcheck="false"
          placeholder="aa:bb:cc:dd:ee:ff"
          class="w-full max-w-[220px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[14px] text-text"
        />
        <span class="mt-1 block text-[12.5px] text-faint">{{ t('boxes.form.wake.macHint') }}</span>
      </label>
      <template v-else-if="wakeMethod === 'extension'">
        <select v-if="wakeActions.length > 0" v-model="wakeTarget" class="mt-2 block max-w-full rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" :aria-label="t('boxes.form.wake.action')">
          <option value="" disabled>{{ t('boxes.form.wake.pick') }}</option>
          <option v-for="a in wakeActions" :key="a.value" :value="a.value">{{ a.label }}</option>
        </select>
        <p v-else class="mt-2 text-[12.5px] text-faint">{{ t('boxes.form.wake.noExtension') }}</p>
        <span class="mt-1 block text-[12.5px] text-faint">{{ t('boxes.form.wake.extensionHint') }}</span>
      </template>
    </fieldset>

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
import { computed, onMounted, ref } from 'vue'
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

const wakeMethod = ref<'' | 'wol' | 'extension'>(props.box?.wakeMethod ?? '')
const wakeMac = ref(props.box?.wakeMac ?? '')
// "extension id|action id" of the chosen action.
const wakeTarget = ref(props.box?.wakeExt ? `${props.box.wakeExt}|${props.box.wakeAction ?? ''}` : '')
const wakeActions = ref<{ value: string; label: string }[]>([])

// The actions extensions offer (a smart plug's switch): any of them can be
// the box's power button.
onMounted(async () => {
  try {
    const res = await api.extensions.list()
    wakeActions.value = res.extensions.flatMap((c) =>
      c.widgets.filter((w) => w.type === 'action' && w.actionId).map((w) => ({ value: `${c.id}|${w.actionId}`, label: `${c.name} · ${w.label}` })),
    )
    // Keep a saved choice visible even if its extension is not running now.
    if (wakeTarget.value && !wakeActions.value.some((a) => a.value === wakeTarget.value)) {
      wakeActions.value.push({ value: wakeTarget.value, label: wakeTarget.value.replace('|', ' · ') })
    }
  } catch {
    // The form works without the list; the choice just stays empty.
  }
})

const valid = computed(
  () =>
    name.value.trim() !== '' &&
    (props.box?.full || url.value.trim() !== '') &&
    (wakeMethod.value !== 'wol' || wakeMac.value.trim() !== '') &&
    (wakeMethod.value !== 'extension' || wakeTarget.value !== ''),
)

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
  const input: BoxInput = { name: name.value.trim(), color: color.value, wakeMethod: wakeMethod.value }
  if (wakeMethod.value === 'wol') input.wakeMac = wakeMac.value.trim()
  if (wakeMethod.value === 'extension') {
    const [ext, action] = wakeTarget.value.split('|')
    input.wakeExt = ext
    input.wakeAction = action
  }
  if (!props.box?.full) Object.assign(input, connection())
  emit('save', input)
}
</script>
