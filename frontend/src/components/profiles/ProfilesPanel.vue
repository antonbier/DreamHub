<!--
  Managing the household's profiles (docs/PROFILES_PLAN.md) — admin only
  (perm "profiles"). A profile is a person with a role; a viewer can have
  two switches flipped away from the role's defaults. Saved directly, like
  settings: DreamHub's own state, recorded in the change log.
-->
<template>
  <section class="rounded border border-line bg-tile p-5">
    <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('profiles.manage.title') }}</h2>
    <p class="mb-4 text-[13px] text-muted">{{ t('profiles.manage.subtitle') }}</p>

    <ul class="mb-4 flex flex-col">
      <li v-for="p in profiles" :key="p.id" class="border-b border-line-soft py-2.5 last:border-none">
        <div class="flex items-center gap-3">
          <ProfileAvatar :name="p.name" :color="p.color" :size="32" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px] font-semibold text-text">{{ p.name }}</span>
            <span class="block text-[12.5px] text-faint">
              {{ t(`profiles.role.${p.role}`) }} · {{ p.hasPin ? t('profiles.manage.withPin') : t('profiles.manage.withoutPin') }}
              <template v-if="differs(p)"> · {{ t('profiles.manage.custom') }}</template>
            </span>
          </span>
          <button type="button" class="shrink-0 text-[13px] text-accent hover:underline" :aria-expanded="editing?.id === p.id" @click="edit(p)">
            {{ t('profiles.manage.edit') }}
          </button>
        </div>
        <ProfileForm
          v-if="editing?.id === p.id"
          class="mt-3"
          :profile="p"
          :colors="colors"
          :self="p.id === me?.id"
          :busy="busy"
          :error="error"
          @save="(input) => save(p.id, input)"
          @remove="remove(p.id)"
          @cancel="editing = null"
        />
      </li>
    </ul>

    <ProfileForm
      v-if="adding"
      :colors="colors"
      :busy="busy"
      :error="error"
      @save="(input) => save(null, input)"
      @cancel="adding = false"
    />
    <button v-else type="button" class="rounded bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg" @click="add">
      {{ t('profiles.manage.add') }}
    </button>
    <p v-if="!adding && !me?.hasPin" class="mt-2 text-[12.5px] text-faint">{{ t('profiles.manage.adminPinFirst') }}</p>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ProfileAvatar from './ProfileAvatar.vue'
import ProfileForm from './ProfileForm.vue'
import { api } from '@/api/client'
import { useAuth } from '@/composables/useAuth'
import { profileErrorKey } from './profileErrors'
import type { Profile, ProfileInput } from '@/api/types'

const { t } = useI18n()
const { me, check } = useAuth()
const profiles = ref<Profile[]>([])
const colors = ref<string[]>([])
const editing = ref<Profile | null>(null)
const adding = ref(false)
const busy = ref(false)
const error = ref('')

async function load() {
  const res = await api.profiles.list()
  profiles.value = res.profiles
  colors.value = res.colors
}
onMounted(() => void load().catch(() => {}))

function differs(p: Profile): boolean {
  return Object.keys(p.overrides).length > 0
}

function edit(p: Profile) {
  adding.value = false
  error.value = ''
  editing.value = editing.value?.id === p.id ? null : p
}

function add() {
  editing.value = null
  error.value = ''
  adding.value = true
}

async function save(id: number | null, input: ProfileInput) {
  busy.value = true
  error.value = ''
  try {
    if (id === null) await api.profiles.create(input)
    else await api.profiles.update(id, input)
    editing.value = null
    adding.value = false
    await load()
    // The first second profile turns the picker on; a changed own
    // profile changes what this device may do.
    await check()
  } catch (err) {
    error.value = t(profileErrorKey(err))
  } finally {
    busy.value = false
  }
}

async function remove(id: number) {
  busy.value = true
  error.value = ''
  try {
    await api.profiles.remove(id)
    editing.value = null
    await load()
    await check()
  } catch (err) {
    error.value = t(profileErrorKey(err))
  } finally {
    busy.value = false
  }
}
</script>
