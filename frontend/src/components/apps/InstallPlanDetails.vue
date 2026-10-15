<!--
  What an install plan will create, exactly (PRINCIPLES.md, "Preview and
  confirm"): per container its image, memory, network, ports, folders on the
  box, variables (secrets hidden) and any extra rights. The backend sends
  this structured; nothing here is guessed.
-->
<template>
  <div class="mt-2">
    <p v-if="after.hostPort" class="text-[12.5px] text-muted">{{ t('apps.plan.reachable', { port: after.hostPort }) }}</p>
    <p v-if="after.storage" class="text-[12.5px] text-muted">{{ t('apps.plan.storageAt', { path: after.storage }) }}</p>
    <details :open="open" class="mt-2">
      <summary class="cursor-pointer text-[12.5px] font-semibold text-muted">{{ t('apps.plan.whatTitle') }}</summary>
      <div class="mt-2 flex flex-col gap-3">
        <section v-for="(c, i) in containers" :key="i" class="rounded-sm border border-line-soft p-2.5 text-[12.5px]">
          <p class="font-semibold text-text">
            {{ c.image }}
            <span v-if="c.service" class="ms-1 font-normal text-muted">({{ serviceLabel(c.service) }})</span>
          </p>
          <dl class="mt-1.5 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-muted">
            <dt>{{ t('apps.plan.memory', { mb: c.memoryMB }) }}</dt>
            <dd class="text-faint">{{ t('apps.plan.networkHost') }}</dd>
            <template v-if="c.ports?.length">
              <dt>{{ t('apps.plan.ports') }}</dt>
              <dd class="text-text">{{ c.ports.map(portText).join(', ') }}</dd>
            </template>
            <template v-if="c.mounts?.length">
              <dt>{{ t('apps.plan.folders') }}</dt>
              <dd class="flex flex-col text-text">
                <span v-for="(m, j) in c.mounts" :key="j" class="break-all">
                  {{ m.host }} → {{ m.container }}<span v-if="m.readOnly" class="text-faint"> · {{ t('apps.plan.readOnly') }}</span>
                </span>
              </dd>
            </template>
            <template v-if="c.env?.length">
              <dt>{{ t('apps.plan.env') }}</dt>
              <dd class="flex flex-col text-text">
                <span v-for="(e, j) in c.env" :key="j" class="break-all">
                  {{ e.key }} =
                  <span v-if="e.masked" class="text-faint">{{ t('apps.plan.hidden') }}</span>
                  <span v-else-if="e.generated" class="text-faint">{{ t('apps.plan.generated') }}</span>
                  <template v-else>{{ e.value }}</template>
                </span>
              </dd>
            </template>
            <template v-if="c.command?.length">
              <dt>{{ t('apps.plan.command') }}</dt>
              <dd class="break-all font-mono text-[11.5px] text-text">{{ c.command.join(' ') }}</dd>
            </template>
            <template v-if="c.elevated">
              <dt class="text-warn">{{ t('apps.plan.elevated') }}</dt>
              <dd class="flex flex-col text-warn">
                <span v-for="(line, j) in elevatedLines(c.elevated)" :key="j">{{ line }}</span>
              </dd>
            </template>
          </dl>
        </section>
      </div>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppForm } from '@/composables/useAppForm'
import type { InstallPreviewContainer, Plan } from '@/api/types'

const props = defineProps<{ plan: Plan; open?: boolean }>()
const { t, te } = useI18n()
const { elevatedText } = useAppForm()

const after = computed(() => (props.plan.changes[0]?.after ?? {}) as { hostPort?: number; storage?: string; containers?: InstallPreviewContainer[] })
const containers = computed(() => after.value.containers ?? [])

function serviceLabel(name: string): string {
  const key = 'apps.service.' + name
  return te(key) ? t(key) : name
}

function portText(p: { port: number; udp?: boolean; local?: boolean }): string {
  const base = p.udp ? `${p.port}/UDP` : String(p.port)
  return p.local ? t('apps.plan.portLocal', { port: base }) : base
}

function elevatedLines(e: NonNullable<InstallPreviewContainer['elevated']>): string[] {
  const out: string[] = []
  if (e.privileged) out.push(elevatedText({ kind: 'privileged' }))
  for (const c of e.capAdd ?? []) out.push(elevatedText({ kind: 'capability', value: c }))
  for (const d of e.devices ?? []) out.push(elevatedText({ kind: 'device', value: d }))
  if (e.pidHost) out.push(elevatedText({ kind: 'pid' }))
  if (e.ipcHost) out.push(elevatedText({ kind: 'ipc' }))
  return out
}
</script>
