<!--
  One profile's fields, for adding or editing (ProfilesPanel.vue): name,
  color, role, PIN, and — for viewers — the two switches the admin may flip
  (backend/internal/perm, Switchable).
-->
<template>
  <form class="rounded-sm border border-line bg-field p-4" @submit.prevent="submit">
    <label class="mb-3 block">
      <span class="mb-1 block text-[13px] text-muted">{{ t('profiles.form.name') }}</span>
      <input v-model="name" type="text" maxlength="30" class="w-full max-w-[280px] rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text" />
    </label>

    <fieldset class="mb-3">
      <legend class="mb-1 text-[13px] text-muted">{{ t('profiles.form.color') }}</legend>
      <div class="flex flex-wrap gap-2">
        <label v-for="c in colors" :key="c" class="cursor-pointer">
          <input v-model="color" type="radio" name="profile-color" :value="c" class="peer sr-only" />
          <span
            class="block h-7 w-7 rounded-full ring-offset-2 ring-offset-field peer-checked:ring-2 peer-checked:ring-text peer-focus-visible:ring-2 peer-focus-visible:ring-accent"
            :style="{ background: `var(--dh-profile-${c})` }"
          />
          <span class="sr-only">{{ t(`profiles.color.${c}`) }}</span>
        </label>
      </div>
    </fieldset>

    <label class="mb-3 block">
      <span class="mb-1 block text-[13px] text-muted">{{ t('profiles.form.role') }}</span>
      <select v-model="role" class="rounded-sm border border-line bg-tile px-2.5 py-1.5 text-[14px] text-text">
        <option value="user">{{ t('profiles.role.user') }}</option>
        <option value="admin">{{ t('profiles.role.admin') }}</option>
      </select>
      <span class="mt-1 block text-[12.5px] text-faint">{{ t(`profiles.roleHint.${role}`) }}</span>
    </label>

    <fieldset v-if="role === 'user'" class="mb-3">
      <legend class="mb-1 text-[13px] text-muted">{{ t('profiles.form.switches') }}</legend>
      <label v-for="s in SWITCHES" :key="s" class="flex items-center gap-2 py-0.5 text-[14px]">
        <input v-model="switches[s]" type="checkbox" />
        {{ t(`profiles.switch.${s}`) }}
        <span v-if="switches[s] !== VIEWER_DEFAULTS[s]" class="text-[12px] text-faint">({{ t('profiles.form.notDefault') }})</span>
      </label>
    </fieldset>

    <fieldset v-if="boxes.length >= 2" class="mb-3">
      <legend class="mb-1 text-[13px] text-muted">{{ t('boxes.profile.legend') }}</legend>
      <label v-for="b in boxes" :key="b.id" class="flex items-center gap-2 py-0.5 text-[14px]">
        <input v-model="allowedBoxes" type="checkbox" :value="b.id" />
        <span class="h-2.5 w-2.5 rounded-full" :style="{ background: `var(--dh-profile-${b.color || 'slate'})` }" aria-hidden="true" />
        {{ b.name }}
      </label>
      <p class="mt-1 text-[12.5px] text-faint">{{ allowedBoxes.length === 0 ? t('boxes.profile.none') : t('boxes.profile.hint') }}</p>
    </fieldset>

    <label class="mb-1 block">
      <span class="mb-1 block text-[13px] text-muted">{{ profile?.hasPin ? t('profiles.form.newPin') : t('profiles.form.pin') }}</span>
      <input
        v-model="pin"
        type="password"
        inputmode="numeric"
        autocomplete="new-password"
        maxlength="8"
        class="w-[160px] rounded-sm border border-line bg-tile px-2.5 py-1.5 font-mono text-[14px] text-text"
      />
    </label>
    <label v-if="profile?.hasPin" class="mb-1 flex items-center gap-2 text-[13.5px]">
      <input v-model="removePin" type="checkbox" />
      {{ t('profiles.form.removePin') }}
    </label>
    <p class="mb-3 text-[12.5px] text-faint">{{ role === 'admin' ? t('profiles.form.adminPinHint') : t('profiles.pinFormat') }}</p>

    <p v-if="error" class="mb-3 text-[13px] text-warn" role="alert">{{ error }}</p>
    <div class="flex flex-wrap items-center gap-2">
      <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="busy || !valid">
        {{ busy ? t('settings.page.saving') : t('profiles.form.save') }}
      </button>
      <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-muted hover:bg-tile hover:text-text" @click="emit('cancel')">
        {{ t('settings.page.cancel') }}
      </button>
      <button
        v-if="profile && !self"
        type="button"
        class="ms-auto rounded-sm px-3 py-1.5 text-[13px]"
        :class="confirmDelete ? 'bg-warn font-semibold text-bg' : 'text-warn hover:bg-tile'"
        :disabled="busy"
        @click="confirmDelete ? emit('remove') : (confirmDelete = true)"
      >
        {{ confirmDelete ? t('profiles.form.confirmDelete', { name: profile.name }) : t('profiles.form.delete') }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import type { BoxInfo, Permission, Profile, ProfileInput, Role } from '@/api/types'

const props = defineProps<{ profile?: Profile; colors: string[]; self?: boolean; busy: boolean; error: string }>()
const emit = defineEmits<{ save: [input: ProfileInput]; remove: []; cancel: [] }>()
const { t } = useI18n()

// The switchable permissions and the viewer role's defaults — mirrors
// backend/internal/perm (Switchable, roles).
const SWITCHES = ['timers.plan', 'recordings.manage'] as const satisfies readonly Permission[]
const VIEWER_DEFAULTS: Record<(typeof SWITCHES)[number], boolean> = { 'timers.plan': true, 'recordings.manage': false }

const name = ref(props.profile?.name ?? '')
const color = ref(props.profile?.color || props.colors[0] || 'blue')
const role = ref<Role>(props.profile?.role ?? 'user')
const pin = ref('')
const removePin = ref(false)
// The boxes the profile may use: none ticked means all of them.
const boxes = ref<BoxInfo[]>([])
const allowedBoxes = ref<string[]>([...(props.profile?.boxes ?? [])])
onMounted(() => {
  void api.boxes.list().then((res) => (boxes.value = res.boxes)).catch(() => {})
})
const confirmDelete = ref(false)
const switches = reactive<Record<(typeof SWITCHES)[number], boolean>>({
  'timers.plan': props.profile?.overrides['timers.plan'] ?? VIEWER_DEFAULTS['timers.plan'],
  'recordings.manage': props.profile?.overrides['recordings.manage'] ?? VIEWER_DEFAULTS['recordings.manage'],
})

const valid = computed(() => name.value.trim() !== '' && (pin.value === '' || /^\d{4,8}$/.test(pin.value)))

function submit() {
  const input: ProfileInput = { name: name.value.trim(), color: color.value, role: role.value }
  // Only what differs from the role's defaults is stored as a switch.
  const overrides: ProfileInput['overrides'] = {}
  if (role.value === 'user') {
    for (const s of SWITCHES) if (switches[s] !== VIEWER_DEFAULTS[s]) overrides[s] = switches[s]
  }
  input.overrides = overrides
  // A single-box household never sees the list, so nothing changes for it.
  if (boxes.value.length >= 2) input.boxes = allowedBoxes.value.length === boxes.value.length ? [] : allowedBoxes.value
  if (pin.value) input.pin = pin.value
  else if (removePin.value) input.pin = ''
  emit('save', input)
}
</script>
