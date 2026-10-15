<!--
  The install form (docs/APPSHUB_PLAN.md, App12), the same for a curated app
  and for an admin's own container. Simple view: the web port, where to
  store the files, and the fields the app requires. Advanced view: every
  field, plus free rows for environment variables, ports and folders. Then
  the usual preview with exactly what will be created, and confirm. Secret
  fields are masked and never appear in the preview.
-->
<template>
  <div>
    <!-- Step 1: the form -->
    <form v-if="step === 'form'" class="flex flex-col gap-3" @submit.prevent="preview">
      <h3 class="text-[14px] font-semibold text-text">{{ t('apps.installTitle', { name: title }) }}</h3>

      <label v-if="source.kind === 'custom'" class="flex flex-col gap-1 text-[13px]">
        <span class="text-muted">{{ t('apps.form.nameLabel') }}</span>
        <input v-model="name" type="text" maxlength="60" class="rounded-sm border border-line bg-field px-2 py-1.5 text-text" />
        <span class="text-[12px] text-faint">{{ t('apps.form.nameHint') }}</span>
      </label>

      <!-- An app whose port can't be moved: say which one it uses. -->
      <p v-if="fixedPort" class="text-[13px] text-muted">
        {{ t('apps.portLabel') }}: <span class="text-text">{{ fixedPort }}</span>
        <span class="block text-[12px] text-faint">{{ t('apps.portFixed') }}</span>
      </p>

      <label v-for="f in visibleFields" :key="f.name" class="flex flex-col gap-1 text-[13px]">
        <span class="text-muted">
          {{ fieldLabel(f) }}<span v-if="f.required" class="ms-1 text-[11.5px] text-faint">({{ t('apps.form.required') }})</span>
        </span>
        <input
          v-model="values[f.name]"
          :type="f.secret ? 'password' : f.type === 'port' ? 'number' : 'text'"
          :min="f.type === 'port' ? (f.webPort ? 1024 : 1) : undefined"
          :max="f.type === 'port' ? 65535 : undefined"
          :required="f.required"
          :autocomplete="f.secret ? 'new-password' : 'off'"
          :placeholder="f.default ?? ''"
          :class="f.type === 'port' ? 'w-32' : 'w-full'"
          class="rounded-sm border border-line bg-field px-2 py-1.5 text-text"
        />
        <span v-if="fieldDescription(f)" class="text-[12px] text-faint">{{ fieldDescription(f) }}</span>
        <span v-if="f.secret" class="text-[12px] text-faint">{{ t('apps.form.secretHint') }}</span>
        <span v-else-if="f.type === 'path'" class="text-[12px] text-faint">{{ t('apps.form.pathHint') }}</span>
      </label>

      <label class="flex flex-col gap-1 text-[13px]">
        <span class="text-muted">{{ t('apps.storageLabel') }}</span>
        <select v-model="storage" class="rounded-sm border border-line bg-field px-2 py-1.5 text-text">
          <option v-for="c in choices" :key="c.path" :value="c.path" :disabled="c.freeBytes < 1 << 30">
            {{ c.label }} — {{ t('apps.storageFree', { size: formatBytes(c.freeBytes, locale) }) }}
          </option>
        </select>
        <span class="text-[12px] text-faint">{{ t('apps.storageHint') }}</span>
      </label>

      <!-- Extra rights: shown up front when the compose file asks for them. -->
      <div v-if="elevatedItems.length > 0" class="rounded-sm border border-warn p-3 text-[12.5px]" role="alert">
        <p class="font-semibold text-warn">{{ t('apps.form.elevatedTitle') }}</p>
        <ul class="mt-1.5 list-disc ps-5 text-text">
          <li v-for="(e, i) in elevatedItems" :key="i">{{ elevatedText(e) }}</li>
        </ul>
        <p class="mt-1.5 text-muted">{{ t('apps.form.elevatedHint') }}</p>
        <label class="mt-2 flex items-center gap-2 text-[13px] text-text">
          <input v-model="confirmElevated" type="checkbox" />
          {{ t('apps.form.elevatedConfirm') }}
        </label>
      </div>

      <button type="button" class="self-start text-[12.5px] text-accent hover:underline" :aria-expanded="advanced" @click="advanced = !advanced">
        {{ advanced ? t('apps.form.advancedHide') : t('apps.form.advancedShow') }}
      </button>

      <div v-if="advanced" class="flex flex-col gap-4 rounded-sm border border-line-soft p-3">
        <p class="text-[12px] text-faint">{{ t('apps.form.extraIntro') }}</p>

        <label class="flex flex-col gap-1 text-[13px]">
          <span class="text-muted">{{ t('apps.form.memoryLabel') }}</span>
          <input v-model="memoryMB" type="number" min="16" class="w-32 rounded-sm border border-line bg-field px-2 py-1.5 text-text" :placeholder="String(defaultMemory)" />
          <span class="text-[12px] text-faint">{{ t('apps.form.memoryHint', { mb: defaultMemory }) }}</span>
        </label>

        <fieldset class="flex flex-col gap-1.5">
          <legend class="text-[13px] font-semibold text-muted">{{ t('apps.form.envTitle') }}</legend>
          <div v-for="(row, i) in extraEnv" :key="i" class="flex items-center gap-2">
            <input v-model="row.key" type="text" :placeholder="t('apps.form.envName')" :aria-label="t('apps.form.envName')" class="w-2/5 rounded-sm border border-line bg-field px-2 py-1.5 text-[13px] text-text" />
            <input v-model="row.value" type="text" :placeholder="t('apps.form.envValue')" :aria-label="t('apps.form.envValue')" class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2 py-1.5 text-[13px] text-text" />
            <button type="button" class="shrink-0 rounded-sm px-2 py-1 text-[13px] text-muted hover:bg-field" :aria-label="t('apps.form.rowRemove')" @click="extraEnv.splice(i, 1)">×</button>
          </div>
          <button type="button" class="self-start text-[12.5px] text-accent hover:underline" @click="extraEnv.push({ key: '', value: '' })">+ {{ t('apps.form.envAdd') }}</button>
        </fieldset>

        <fieldset class="flex flex-col gap-1.5">
          <legend class="text-[13px] font-semibold text-muted">{{ t('apps.form.portsTitle') }}</legend>
          <p class="text-[12px] text-faint">{{ t('apps.form.portsHint') }}</p>
          <div v-for="(row, i) in extraPorts" :key="i" class="flex items-center gap-2">
            <input v-model="row.port" type="number" min="1" max="65535" :placeholder="t('apps.form.portNumber')" :aria-label="t('apps.form.portNumber')" class="w-28 rounded-sm border border-line bg-field px-2 py-1.5 text-[13px] text-text" />
            <label class="flex items-center gap-1 text-[13px] text-muted"><input v-model="row.udp" type="checkbox" /> {{ t('apps.form.portUdp') }}</label>
            <button type="button" class="shrink-0 rounded-sm px-2 py-1 text-[13px] text-muted hover:bg-field" :aria-label="t('apps.form.rowRemove')" @click="extraPorts.splice(i, 1)">×</button>
          </div>
          <button type="button" class="self-start text-[12.5px] text-accent hover:underline" @click="extraPorts.push({ port: '', udp: false })">+ {{ t('apps.form.portAdd') }}</button>
        </fieldset>

        <fieldset class="flex flex-col gap-1.5">
          <legend class="text-[13px] font-semibold text-muted">{{ t('apps.form.pathsTitle') }}</legend>
          <p class="text-[12px] text-faint">{{ t('apps.form.pathsHint') }}</p>
          <div v-for="(row, i) in extraPaths" :key="i" class="flex flex-wrap items-center gap-2">
            <input v-model="row.host" type="text" :placeholder="t('apps.form.pathHost')" :aria-label="t('apps.form.pathHost')" class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2 py-1.5 text-[13px] text-text" />
            <input v-model="row.container" type="text" :placeholder="t('apps.form.pathContainer')" :aria-label="t('apps.form.pathContainer')" class="min-w-0 flex-1 rounded-sm border border-line bg-field px-2 py-1.5 text-[13px] text-text" />
            <label class="flex items-center gap-1 text-[13px] text-muted"><input v-model="row.readOnly" type="checkbox" /> {{ t('apps.form.pathReadOnly') }}</label>
            <button type="button" class="shrink-0 rounded-sm px-2 py-1 text-[13px] text-muted hover:bg-field" :aria-label="t('apps.form.rowRemove')" @click="extraPaths.splice(i, 1)">×</button>
          </div>
          <button type="button" class="self-start text-[12.5px] text-accent hover:underline" @click="extraPaths.push({ host: '', container: '', readOnly: false })">+ {{ t('apps.form.pathAdd') }}</button>
        </fieldset>
      </div>

      <ul v-if="issues.length > 0" class="flex flex-col gap-1 text-[12.5px] text-warn" role="alert">
        <li v-for="(i, n) in issues" :key="n">{{ issueText(i) }}</li>
      </ul>
      <p v-else-if="error" class="text-[12.5px] text-warn" role="alert">{{ error }}</p>

      <div class="flex justify-end gap-2">
        <button type="button" class="rounded-sm px-3 py-1.5 text-[13px] text-text hover:bg-field" @click="emit('cancel')">{{ t('plan.cancel') }}</button>
        <button type="submit" class="rounded-sm bg-accent px-3 py-1.5 text-[13px] font-semibold text-bg disabled:opacity-60" :disabled="elevatedItems.length > 0 && !confirmElevated">
          {{ t('apps.installPreview') }}
        </button>
      </div>
    </form>

    <p v-else-if="step === 'loading'" class="text-[13px] text-faint">{{ t('common.loading') }}</p>

    <!-- Step 2: exactly what will be created -->
    <PlanPreview
      v-else-if="step === 'plan' && plan"
      :plan="plan"
      :confirm-label="t('apps.installConfirm')"
      :busy="applying"
      :error="error"
      @confirm="confirm"
      @cancel="step = 'form'"
    >
      {{ t('apps.plan.summary', { name: title, mb: totalMB }) }}
      <InstallPlanDetails :plan="plan" :open="source.kind === 'custom'" />
    </PlanPreview>

    <p v-else-if="step === 'done'" class="flex items-center gap-1.5 text-[13px] text-ok" role="status">
      <span class="h-1.5 w-1.5 rounded-full bg-ok" />{{ t('apps.installed') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '@/api/client'
import PlanPreview from '@/components/PlanPreview.vue'
import InstallPlanDetails from '@/components/apps/InstallPlanDetails.vue'
import { useAppForm } from '@/composables/useAppForm'
import { formatBytes } from '@/lib/format'
import type { AppField, AppStorageChoice, CatalogApp, ComposeIssue, ElevatedItem, InstallRequest, ParsedApp, Plan } from '@/api/types'

const props = defineProps<{ source: { kind: 'catalog'; app: CatalogApp } | { kind: 'custom'; app: ParsedApp } }>()
const emit = defineEmits<{ done: []; cancel: [] }>()

const { t, locale } = useI18n()
const { issueText, fieldLabel, fieldDescription, elevatedText, failure } = useAppForm()

const step = ref<'form' | 'loading' | 'plan' | 'done'>('form')
const plan = ref<Plan | null>(null)
const applying = ref(false)
const error = ref('')
const issues = ref<ComposeIssue[]>([])

const fields = computed<AppField[]>(() => props.source.app.fields)
const name = ref(props.source.kind === 'custom' ? props.source.app.name : '')
const title = computed(() => (props.source.kind === 'custom' ? name.value.trim() || props.source.app.name : props.source.app.name))
const advanced = ref(false)

// Field values start at their defaults; only what differs is sent.
const values = reactive<Record<string, string>>({})
for (const f of fields.value) values[f.name] = f.default ?? ''

const visibleFields = computed(() => fields.value.filter((f) => advanced.value || !f.advanced))
// A fixed web port (no field for it): shown as information.
const fixedPort = computed(() => {
  if (fields.value.some((f) => f.webPort)) return 0
  return props.source.kind === 'catalog' ? (props.source.app.port ?? 0) : (props.source.app.webPort ?? 0)
})
const defaultMemory = computed(() => props.source.app.memoryMB || 256)
const memoryMB = ref('')
const totalMB = computed(() => {
  const items = plan.value?.changes[0]?.after as { memoryLimitMB?: number } | undefined
  return items?.memoryLimitMB ?? defaultMemory.value
})

const extraEnv = reactive<{ key: string; value: string }[]>([])
const extraPorts = reactive<{ port: string; udp: boolean }[]>([])
const extraPaths = reactive<{ host: string; container: string; readOnly: boolean }[]>([])

const elevatedItems = ref<ElevatedItem[]>(props.source.kind === 'custom' ? props.source.app.elevated : [])
const confirmElevated = ref(false)

const choices = ref<AppStorageChoice[]>([])
const storage = ref('')
onMounted(async () => {
  try {
    choices.value = (await api.apps.storage()).choices
  } catch {
    // no choice offered: the default location is used
  }
  storage.value = (choices.value.find((c) => c.default) ?? choices.value[0])?.path ?? ''
})

function request(): InstallRequest {
  const req: InstallRequest = {}
  if (props.source.kind === 'catalog') req.appId = props.source.app.id
  else req.custom = { compose: props.source.app.compose, name: name.value.trim() }
  const changed: Record<string, string> = {}
  for (const f of fields.value) {
    const v = (values[f.name] ?? '').trim()
    if (v !== '' && v !== (f.default ?? '')) changed[f.name] = v
    // A required field without a default is sent as typed (possibly empty:
    // the server says which one is missing).
    else if (f.required && !f.default) changed[f.name] = v
  }
  if (Object.keys(changed).length > 0) req.values = changed
  const defaultStorage = choices.value.find((c) => c.default)?.path
  if (storage.value && storage.value !== defaultStorage) req.storage = storage.value
  const mem = parseInt(memoryMB.value, 10)
  if (mem > 0) req.memoryLimitMB = mem
  const env = extraEnv.filter((r) => r.key.trim() !== '').map((r) => ({ key: r.key.trim(), value: r.value }))
  const ports = extraPorts.filter((r) => r.port !== '').map((r) => ({ port: Number(r.port), udp: r.udp }))
  const paths = extraPaths.filter((r) => r.host.trim() !== '' || r.container.trim() !== '').map((r) => ({ host: r.host.trim(), container: r.container.trim(), readOnly: r.readOnly }))
  if (env.length + ports.length + paths.length > 0) req.extra = { env, ports, paths }
  if (confirmElevated.value) req.confirmElevated = true
  return req
}

async function preview() {
  error.value = ''
  issues.value = []
  step.value = 'loading'
  try {
    plan.value = (await api.apps.installPlan(request())).plan
    step.value = 'plan'
  } catch (err) {
    const f = failure(err)
    error.value = f.message
    issues.value = f.issues
    if (f.elevated.length > 0) {
      elevatedItems.value = f.elevated
      confirmElevated.value = false
    }
    step.value = 'form'
  }
}

async function confirm() {
  if (!plan.value) return
  applying.value = true
  error.value = ''
  try {
    await api.plans.apply(plan.value.id)
    step.value = 'done'
    emit('done')
  } catch (err) {
    const f = failure(err)
    error.value = f.message
  } finally {
    applying.value = false
  }
}
</script>
