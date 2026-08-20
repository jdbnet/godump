<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  Menu, X, LayoutDashboard, Database, Archive, ScrollText, LogOut, Sun, Moon, Play,
} from '@lucide/vue'
import { useAuthStore } from '@/stores/auth'
import { applyTheme, useThemeStore } from '@/stores/theme'
import { useBackupsStore } from '@/stores/backups'
import ConfirmDialog from '@/components/ConfirmDialog.vue'

const route = useRoute()
const auth = useAuthStore()
const theme = useThemeStore()
const backups = useBackupsStore()
const sidebarOpen = ref(false)

const nav = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, match: (p: string) => p === '/' },
  { to: '/instances', label: 'Instances', icon: Database, match: (p: string) => p.startsWith('/instances') },
  { to: '/backups', label: 'Backups', icon: Archive, match: (p: string) => p.startsWith('/backups') },
  { to: '/logs', label: 'Logs', icon: ScrollText, match: (p: string) => p.startsWith('/logs') },
]

const pageTitle = computed(() => nav.find((n) => n.match(route.path))?.label || 'GoDump')

onMounted(() => {
  applyTheme(theme.dark)
})

async function logout() {
  await auth.logout()
  window.location.href = '/login'
}

async function runAll() {
  await backups.runAll()
}
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-surface">
    <div v-if="sidebarOpen" class="fixed inset-0 z-40 bg-black/50 lg:hidden" @click="sidebarOpen = false" />
    <aside
      class="fixed inset-y-0 left-0 z-50 flex h-screen w-64 shrink-0 flex-col border-r border-default bg-surface-raised transition-transform lg:static lg:translate-x-0"
      :class="sidebarOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="flex items-center gap-3 border-b border-default p-4">
        <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-accent/20 text-sm font-bold text-accent">GD</div>
        <div class="min-w-0 flex-1">
          <div class="truncate text-sm font-semibold text-heading">GoDump</div>
          <div class="text-xs text-muted">MariaDB backups</div>
        </div>
        <button type="button" class="text-muted lg:hidden" @click="sidebarOpen = false"><X class="h-5 w-5" /></button>
      </div>
      <nav class="flex-1 overflow-y-auto p-2">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="mb-0.5 flex items-center gap-2 rounded-lg px-3 py-2 text-sm transition"
          :class="item.match(route.path) ? 'nav-item-active' : 'nav-item-inactive'"
          @click="sidebarOpen = false"
        >
          <component :is="item.icon" class="h-4 w-4 shrink-0" />
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="space-y-2 border-t border-default p-3">
        <button
          type="button"
          class="btn-primary w-full"
          :disabled="backups.actionBusy || backups.status?.any_running"
          @click="runAll"
        >
          <Play class="h-4 w-4" />
          Run All Now
        </button>
        <div class="flex items-stretch gap-1.5">
          <button type="button" class="btn-secondary shrink-0 px-2.5 py-2" @click="theme.toggle()">
            <Sun v-if="theme.dark" class="h-4 w-4" />
            <Moon v-else class="h-4 w-4" />
          </button>
          <button
            v-if="auth.authEnabled"
            type="button"
            class="btn-secondary min-w-0 flex-1"
            @click="logout"
          >
            <LogOut class="h-4 w-4 shrink-0" />
            Sign out
          </button>
        </div>
      </div>
    </aside>
    <div class="flex min-w-0 flex-1 flex-col">
      <header class="flex items-center gap-3 border-b border-default bg-surface-raised px-4 py-3 lg:hidden">
        <button type="button" class="text-muted" @click="sidebarOpen = true"><Menu class="h-5 w-5" /></button>
        <h1 class="text-sm font-semibold text-heading">{{ pageTitle }}</h1>
      </header>
      <main class="flex-1 overflow-y-auto p-4 lg:p-6">
        <slot />
      </main>
    </div>
    <ConfirmDialog />
  </div>
</template>
