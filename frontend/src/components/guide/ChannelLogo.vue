<!--
  A channel's logo (picon) from the box (GET /api/v1/guide/picon, or
  /api/v1/radio/picon for a radio station, /api/v1/stream/picon in
  StreamHub), fetched
  with the app's auth header and shared per channel for the session.
  Renders nothing while loading, and the channel's initials when there is
  no logo — the channel's name next to it carries the meaning, so the logo
  is decorative (alt="").
-->
<template>
  <img v-if="src" :src="src" alt="" class="shrink-0 object-contain" :style="{ height: `${height}px`, maxWidth: `${height * 2.2}px` }" />
  <!-- No logo for this channel: its initials, so the row keeps its shape. -->
  <span
    v-else-if="missed"
    aria-hidden="true"
    class="flex shrink-0 items-center justify-center rounded bg-tile-2 font-semibold text-muted"
    :style="{ height: `${height}px`, width: `${height}px`, fontSize: `${Math.max(9, Math.round(height * 0.45))}px` }"
    >{{ initials }}</span
  >
</template>

<script lang="ts">
import { ref as vueRef } from 'vue'
import { api, displayRef } from '@/api/client'

const cache = new Map<string, Promise<string | null>>()
// Bumped when someone chooses another logo by hand, so every logo on
// screen asks again instead of showing what it remembered.
const revision = vueRef(0)
export function invalidateLogos() {
  cache.clear()
  revision.value++
}
// source: which hub serves the logo — GuideHub's for TV channels,
// RadioHub's for stations, StreamHub's and ChannelHub's for their own
// lists — so no screen depends on another hub.
function piconURL(ref: string, name: string, source: 'guide' | 'radio' | 'stream' | 'channels') {
  // Per channel: its display reference (never credentials) and its name.
  const key = `${source}|${displayRef(ref)}|${name}`
  let p = cache.get(key)
  if (!p) {
    p = api[source]
      .picon(ref, name)
      .then((b) => URL.createObjectURL(b))
      .catch(() => null) // none: remembered, not asked again this session
    cache.set(key, p)
  }
  return p
}
</script>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ serviceReference: string; serviceName: string; height?: number; source?: 'guide' | 'radio' | 'stream' | 'channels' }>(), {
  height: 20,
  source: 'guide',
})
const src = ref<string | null>(null)
const missed = ref(false)
const initials = computed(() => {
  const words = props.serviceName.replace(/[^\p{L}\p{N} ]/gu, ' ').split(/\s+/).filter(Boolean)
  const two = words.length > 1 ? words[0][0] + words[1][0] : (words[0] ?? '?').slice(0, 2)
  return two.toLocaleUpperCase()
})
watch(
  () => [props.serviceReference, revision.value] as const,
  async ([ref]) => {
    src.value = null
    missed.value = false
    const url = await piconURL(ref, props.serviceName, props.source)
    if (props.serviceReference !== ref) return
    src.value = url
    missed.value = url === null
  },
  { immediate: true },
)
</script>
