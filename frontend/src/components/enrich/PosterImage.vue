<!--
  A poster or backdrop: a recording's own cover art already on the box
  (`recording`, GET /api/v1/media/cover — preferred, it's what the box's
  other plugins show too), else one from enrichment (`path`, TMDB through
  DreamHub's image endpoint). Fetched with the app's auth header (an
  <img src> can't carry it) and shown as an object URL. Object URLs are shared per image for the
  session, so the same poster in two places is fetched once. Without a
  path, or while loading, it's a plain `field`-colored box of the same
  size, so rows don't jump when the picture arrives.
-->
<template>
  <span
    class="block shrink-0 overflow-hidden rounded-sm bg-field"
    :class="{ 'h-full w-full': fill }"
    :style="fill ? undefined : { width: `${width}px`, height: `${height}px` }"
  >
    <img v-if="src" :src="src" :alt="alt" class="h-full w-full object-cover" loading="lazy" />
  </span>
</template>

<script lang="ts">
import { api } from '@/api/client'

// Session-wide cache: image key → object URL (or the pending fetch).
const urls = new Map<string, Promise<string | null>>()

function load(key: string, fetchBlob: () => Promise<Blob>): Promise<string | null> {
  let p = urls.get(key)
  if (!p) {
    p = fetchBlob()
      .then((blob) => URL.createObjectURL(blob))
      .catch(() => {
        urls.delete(key) // try again another time
        return null
      })
    urls.set(key, p)
  }
  return p
}
</script>

<script setup lang="ts">
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    path?: string
    // A recording's .ts path whose own cover art to show instead of path.
    recording?: string
    // A series (marker id) whose folder cover to show instead of path.
    series?: number
    size?: 'w92' | 'w185' | 'w342' | 'w780'
    width?: number
    height?: number
    alt?: string
    // Fill the parent instead of a fixed size (a backdrop header).
    fill?: boolean
  }>(),
  { size: 'w92', width: 36, height: 52, alt: '' },
)

const src = ref<string | null>(null)
watch(
  () => [props.recording, props.series, props.path, props.size],
  async () => {
    src.value = null
    const { recording, series, path, size } = props
    let url: string | null = null
    if (recording) url = await load(`rec:${recording}`, () => api.media.cover(recording))
    else if (series !== undefined) url = await load(`series:${series}`, () => api.series.cover(series))
    else if (path) url = await load(`${size}${path}`, () => api.enrich.image(size, path))
    else return
    if (props.recording === recording && props.series === series && props.path === path) src.value = url
  },
  { immediate: true },
)
</script>
