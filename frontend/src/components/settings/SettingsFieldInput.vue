<!--
  One settings field, rendered from its schema (backend/internal/settings.Field).
  The control follows `field.type`; secret/password fields clear themselves
  on focus so typing always replaces rather than edits the masked
  placeholder — see the focus/blur handlers below.
-->
<template>
  <div class="grid grid-cols-[220px_1fr] items-start gap-4 below-md:grid-cols-1 below-md:gap-1.5">
    <label :for="inputId" class="pt-1.5 text-[14px] text-muted">{{ label }}</label>
    <div class="max-w-[420px]">
      <!-- bool -->
      <button
        v-if="field.type === 'bool'"
        :id="inputId"
        type="button"
        role="switch"
        :aria-checked="Boolean(modelValue)"
        class="flex h-6 w-10 items-center rounded-full p-0.5 transition-colors"
        :class="modelValue ? 'bg-accent justify-end' : 'bg-field justify-start'"
        @click="$emit('update:modelValue', !modelValue)"
      >
        <span class="h-5 w-5 rounded-full bg-tile shadow" />
      </button>

      <!-- enum -->
      <select
        v-else-if="field.type === 'enum'"
        :id="inputId"
        class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        :value="modelValue"
        @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="opt in field.options ?? []" :key="opt" :value="opt">{{ optionLabel(field, opt) }}</option>
      </select>

      <!-- int -->
      <input
        v-else-if="field.type === 'int'"
        :id="inputId"
        type="number"
        :min="field.min"
        :max="field.max"
        class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        :value="modelValue as number"
        @input="$emit('update:modelValue', Number(($event.target as HTMLInputElement).value))"
      />

      <!-- secret / password: clear on focus if untouched, so typing always
           starts fresh rather than editing the masked text or a stale draft. -->
      <input
        v-else-if="field.type === 'secret' || field.type === 'password'"
        :id="inputId"
        type="password"
        autocomplete="new-password"
        :placeholder="secretPlaceholder"
        class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        :value="(modelValue as string) ?? ''"
        @focus="onSecretFocus"
        @blur="onSecretBlur"
        @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      />

      <!-- string / timezone -->
      <input
        v-else
        :id="inputId"
        type="text"
        :placeholder="field.type === 'timezone' ? 'Europe/Rome' : undefined"
        class="w-full rounded border border-line bg-field px-2.5 py-1.5 text-[14px] text-text"
        :value="(modelValue as string) ?? ''"
        @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      />

      <p v-if="field.type === 'timezone'" class="mt-1 text-[12.5px] text-faint">{{ t('settings.page.timezoneHint') }}</p>
      <p v-else-if="field.key === 'network.allowedHosts'" class="mt-1 text-[12.5px] text-faint">
        {{ t('settings.page.allowedHostsHint') }}
      </p>
      <p v-else-if="field.key === 'push.server'" class="mt-1 text-[12.5px] text-faint">
        {{ t('settings.page.pushServerHint') }}
      </p>
      <p v-else-if="field.key.startsWith('media.naming.folder')" class="mt-1 text-[12.5px] text-faint">
        {{ t('settings.page.folderHint') }}
      </p>
      <p v-else-if="field.key.startsWith('media.naming.')" class="mt-1 text-[12.5px] text-faint">
        {{ t('settings.page.namingHint') }}
      </p>
      <p v-else-if="field.type === 'secret' || field.type === 'password'" class="mt-1 text-[12.5px] text-faint">
        {{ statusHint }}
      </p>

      <p v-if="error" class="mt-1 text-[12.5px] text-warn">{{ errorText }}</p>

      <div class="mt-1 flex gap-3">
        <button
          v-if="!field.isDefault && field.type !== 'password'"
          type="button"
          class="text-[12.5px] text-accent hover:underline"
          @click="$emit('reset')"
        >
          {{ resetLabel }}
        </button>
        <button
          v-if="field.type === 'password' && field.hasValue"
          type="button"
          class="text-[12.5px] text-accent hover:underline"
          @click="$emit('reset')"
        >
          {{ t('settings.page.clear') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSettingsText } from '@/composables/useSettingsText'
import type { FieldError, SettingField } from '@/api/types'

const props = defineProps<{
  field: SettingField
  modelValue: unknown
  // The pristine (last-loaded) value — restored on an empty blur so a mere
  // focus-then-click-away on a secret/password field never counts as a
  // change. See onSecretBlur.
  original: unknown
  error?: FieldError
}>()
const emit = defineEmits<{ 'update:modelValue': [unknown]; reset: [] }>()

const { t } = useI18n()
const { fieldLabel, optionLabel, fieldError } = useSettingsText()

const inputId = `setting-${props.field.key}`
const label = computed(() => fieldLabel(props.field))
const errorText = computed(() => (props.error ? fieldError(props.error) : ''))
const resetLabel = computed(() =>
  props.field.type === 'secret' ? t('settings.page.clear') : t('settings.page.resetField'),
)
const secretPlaceholder = computed(() =>
  props.field.type === 'password' ? t('settings.page.passwordPlaceholder') : t('settings.page.secretPlaceholder'),
)
const statusHint = computed(() => (props.field.hasValue ? t('settings.page.isSet') : t('settings.page.notSet')))

// If the field is still exactly what it was on load (pristine — whether
// that's a masked secret, or nothing at all), clicking in clears it so the
// user always types a fresh value rather than appending to a mask or a
// leftover draft. Clicking away again without typing restores that
// pristine state instead of accidentally clearing a real secret.
function onSecretFocus(e: FocusEvent) {
  const input = e.target as HTMLInputElement
  if (input.value !== '') {
    input.value = ''
    emit('update:modelValue', '')
  }
}
function onSecretBlur(e: FocusEvent) {
  const input = e.target as HTMLInputElement
  if (input.value === '') {
    emit('update:modelValue', props.original)
  }
}
</script>
