<!--
  A channel's logo (picon) from the box (GET /api/v1/guide/picon), fetched
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
function piconURL(ref: string, name: string) {
  let p = cache.get(ref)
  if (!p) {
    p = api.guide
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

const props = withDefaults(defineProps<{ serviceReference: string; serviceName: string; height?: number }>(), { height: 20 })
const src = ref<string | null>(null)
watch(
  () => props.serviceReference,
  async (ref) => {
    src.value = null
    const url = await piconURL(ref, props.serviceName)
    if (props.serviceReference === ref) src.value = url
  },
  { immediate: true },
)
</script>
