<!--
  ControlHub (CONCEPT.md: "Web remote, message to TV, screenshot, power"),
  pulled forward from v1.1 into v1 (docs/archive/V1_PLAN.md) — verifying WP8's
  Enigma2 plugin needed a real screenshot/remote-control loop anyway, and
  GuideHub's "switch to this channel now" needs it too. Every action here
  is direct and immediate, not preview-and-confirm (PRINCIPLES.md): a
  virtual remote is inherently a live, instantly-reversible input device,
  the same way a physical remote (or WebControl's own native remote page,
  which DreamHub deliberately doesn't reimplement) works — see
  backend/internal/hubs/control.go's own doc comment.

  The picture and the remote sit side by side, so a key's result is visible
  right next to the key (the picture refreshes itself after each one); they
  stack below 900px, with the picture pinned to the top while scrolling.
  The same two components also make up the side panel every other page can
  open (components/control/RemotePanel.vue).
-->
<template>
  <div class="mx-auto max-w-content p-6 below-md:p-4">
    <div class="flex items-center gap-3">
      <Icon name="control" :size="24" class="text-muted" />
      <h1 class="text-[26px] font-bold">{{ hubTitle('control', 'hub.control') }}</h1>
    </div>

    <HubStatusBanner :item="item" />

    <div
      v-if="!item || item.status !== 'unavailable'"
      class="mt-5 grid grid-cols-[minmax(0,1fr)_300px] items-start gap-5 below-md:grid-cols-1"
    >
      <!-- Stays in view while scrolling down to the remote on a phone. -->
      <div class="flex flex-col gap-5">
        <div class="rounded-sm border border-line bg-tile p-3 below-md:sticky below-md:top-0 below-md:z-10">
          <ScreenView />
        </div>
        <div class="rounded-sm border border-line bg-tile p-4 below-md:hidden">
          <TvMessage />
        </div>
      </div>
      <div class="rounded-sm border border-line bg-tile p-4">
        <RemotePad />
      </div>
      <!-- Below 900px the message form follows the remote instead. -->
      <div class="hidden rounded-sm border border-line bg-tile p-4 below-md:block">
        <TvMessage />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import HubStatusBanner from '@/components/HubStatusBanner.vue'
import { useHubLabel } from '@/composables/useHubLabel'
import Icon from '@/components/Icon.vue'
import RemotePad from '@/components/control/RemotePad.vue'
import ScreenView from '@/components/control/ScreenView.vue'
import TvMessage from '@/components/control/TvMessage.vue'
import { useControlHub } from '@/composables/useControlHub'

const { hubTitle } = useHubLabel()
const { item } = useControlHub()
</script>
