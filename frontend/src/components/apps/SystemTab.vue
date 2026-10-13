<!--
  AppsHub's "System" tab: the box's own software (DreamOS packages — TV
  plugins, skins, tools) and where it comes from. It lives next to Apps
  and Extensions because it's the same question — "what can I add?" —
  just for the box itself rather than a container or a DreamHub add-on.
-->
<template>
  <div>
    <PackagesPanel :key="reloadKey" />
    <FeedsPanel @changed="reloadKey++" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import FeedsPanel from '@/components/apps/FeedsPanel.vue'
import PackagesPanel from '@/components/apps/PackagesPanel.vue'

const reloadKey = ref(0)
</script>
