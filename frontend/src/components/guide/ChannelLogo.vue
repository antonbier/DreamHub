<!--
  A channel's logo (picon) from the box (GET /api/v1/guide/picon, or
  /api/v1/radio/picon for a radio station), fetched
  with the app's auth header and shared per channel for the session.
  Renders nothing while loading or when the box has none — the channel's
  name next to it carries the meaning, so the logo is decorative (alt="").
-->
<template>
  <img v-if="src" :src="src" alt="" class="shrink-0 object-contain" :style="{ height: `${height}px`, maxWidth: `${height * 2.2}px` }" />
</template>

<script lang="ts">
import { api } from '@/api/client'

const cache = new Map<string, Promise<string | null>>()
// source: which hub serves the logo — GuideHub's for TV channels,
// RadioHub's for stations, so neither screen depends on the other hub.
function piconURL(ref: string, name: string, source: 'guide' | 'radio') {
  let p = cache.get(ref)
  if (!p) {
    p = api[source]
      .picon(ref, name)
      .then((b) => URL.createObjectURL(b))
      .catch(() => null) // none: remembered, not asked again this session
    cache.set(ref, p)
  }
  return p
}
</script>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(defineProps<{ serviceReference: string; serviceName: string; height?: number; source?: 'guide' | 'radio' }>(), {
  height: 20,
  source: 'guide',
})
const src = ref<string | null>(null)
watch(
  () => props.serviceReference,
  async (ref) => {
    src.value = null
    const url = await piconURL(ref, props.serviceName, props.source)
    if (props.serviceReference === ref) src.value = url
  },
  { immediate: true },
)
</script>
