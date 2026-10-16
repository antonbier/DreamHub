<!--
  The boxes DreamHub controls (docs/MULTIDEVICE_PLAN.md, M3) — admin only.
  The first box is the one DreamHub was set up with; its address and login
  stay under Services → WebControl. Further boxes are added by address and
  the login of their web interface; "Test" tries it before anything is
  saved. Removing a box is previewed first, like every change.
-->
<template>
  <section class="rounded-sm border border-line bg-tile p-5">
    <h2 class="mb-1 text-[15.5px] font-semibold">{{ t('boxes.manage.title') }}</h2>
    <p class="mb-4 text-[13px] text-muted">{{ t('boxes.manage.subtitle') }}</p>

    <ul class="mb-4 flex flex-col">
      <li v-for="b in boxes" :key="b.id" class="border-b border-line-soft py-2.5 last:border-none">
        <div class="flex items-center gap-3">
          <span class="h-3 w-3 shrink-0 rounded-full" :style="{ background: `var(--dh-profile-${b.color || 'slate'})` }" aria-hidden="true" />
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[14px] font-semibold text-text">{{ b.name }}</span>
            <span class="block truncate text-[12.5px] text-faint">
              {{ b.full ? t('boxes.manage.thisMachine') : b.url }}
              <template v-if="b.paired"> · {{ t('boxes.manage.paired') }}</template>
              <template v-if="!b.online"> · {{ t('boxes.selector.offline') }}</template>
            </span>
          </span>
          <button type="button" class="shrink-0 text-[13px] text-accent hover:underline" :aria-expanded="editing === b.id" @click="edit(b.id)">
            {{ t('boxes.manage.edit') }}
          </button>
        </div>
        <template v-if="editing === b.id">
          <BoxForm
            class="mt-3"
            :box="b"
            :busy="busy"
            :error="error"
            @save="(input) => save(b.id, input)"
            @remove="startRemove(b)"
            @cancel="editing = ''"
          />
          <BoxPairing v-if="!b.full" class="mt-3" :box="b" @changed="load" />
          <PlanPreview
            v-if="removal && removal.id === b.id"
            class="mt-3"
            :plan="removal.plan"
            :confirm-label="t('boxes.manage.removeConfirm')"
            :busy="busy"
            :error="error"
            danger
            @confirm="applyRemoval"
            @cancel="removal = null"
          >
            {{ t('boxes.manage.removeText', { name: b.name }) }}
          </PlanPreview>
        </template>
      </li>
    </ul>

    <BoxForm v-if="adding" :busy="busy" :error="error" @save="(input) => save(null, input)" @cancel="adding = false" />
    <button v-else type="button" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg" @click="add">
      {{ t('boxes.manage.add') }}
    </button>
    <p v-if="!adding" class="mt-2 text-[12.5px] text-faint">{{ t('boxes.manage.basicHint') }}</p>
    <PairThisBox />
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BoxForm from './BoxForm.vue'
import BoxPairing from './BoxPairing.vue'
import PairThisBox from './PairThisBox.vue'
import PlanPreview from '@/components/PlanPreview.vue'
import { api } from '@/api/client'
import { useBoxesStore } from '@/stores/boxes'
import { boxErrorKey } from './boxErrors'
import type { BoxInfo, BoxInput, Plan } from '@/api/types'

const { t } = useI18n()
const store = useBoxesStore()
const boxes = ref<BoxInfo[]>([])
const editing = ref('')
const adding = ref(false)
const busy = ref(false)
const error = ref('')
const removal = ref<{ id: string; plan: Plan } | null>(null)

async function load() {
  const res = await api.boxes.list()
  boxes.value = res.boxes
  // The selector and the rest of the app follow the change.
  await store.refresh()
}
onMounted(() => void load().catch(() => {}))

function edit(id: string) {
  adding.value = false
  error.value = ''
  removal.value = null
  editing.value = editing.value === id ? '' : id
}

function add() {
  editing.value = ''
  error.value = ''
  adding.value = true
}

async function save(id: string | null, input: BoxInput) {
  busy.value = true
  error.value = ''
  try {
    if (id === null) await api.boxes.add(input)
    else await api.boxes.update(id, input)
    editing.value = ''
    adding.value = false
    await load()
  } catch (err) {
    error.value = t(boxErrorKey(err))
  } finally {
    busy.value = false
  }
}

async function startRemove(b: BoxInfo) {
  busy.value = true
  error.value = ''
  try {
    removal.value = { id: b.id, plan: (await api.boxes.planRemove(b.id)).plan }
  } catch (err) {
    error.value = t(boxErrorKey(err))
  } finally {
    busy.value = false
  }
}

async function applyRemoval() {
  if (!removal.value) return
  busy.value = true
  error.value = ''
  try {
    await api.plans.apply(removal.value.plan.id)
    removal.value = null
    editing.value = ''
    await load()
  } catch (err) {
    error.value = t(boxErrorKey(err))
  } finally {
    busy.value = false
  }
}
</script>
