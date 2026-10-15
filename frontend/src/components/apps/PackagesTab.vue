<!--
  The "Packages" tab: the box's own software (DreamOS packages — TV
  plugins, skins, tools). Where they come from (the feeds) is behind the
  gear: switching one on or off and adding one are rare.
-->
<template>
  <div>
    <TabIntro :text="t('apps.intro.packages')" :gear-label="t('apps.feeds.gear')" @gear="feedsOpen = true" />
    <PackagesPanel :key="reloadKey" />
    <SettingsDialog v-if="feedsOpen" :title="t('apps.feeds.title')" @close="feedsOpen = false">
      <FeedsPanel @changed="reloadKey++" />
      <AutoUpdateSettings kind="packages" />
    </SettingsDialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AutoUpdateSettings from '@/components/apps/AutoUpdateSettings.vue'
import FeedsPanel from '@/components/apps/FeedsPanel.vue'
import PackagesPanel from '@/components/apps/PackagesPanel.vue'
import SettingsDialog from '@/components/apps/SettingsDialog.vue'
import TabIntro from '@/components/apps/TabIntro.vue'

const { t } = useI18n()
const reloadKey = ref(0)
const feedsOpen = ref(false)
</script>
