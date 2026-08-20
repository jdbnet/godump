import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import LoginView from '@/views/LoginView.vue'
import DashboardView from '@/views/DashboardView.vue'
import InstancesView from '@/views/InstancesView.vue'
import BackupsView from '@/views/BackupsView.vue'
import LogsView from '@/views/LogsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: LoginView, meta: { public: true } },
    { path: '/', component: DashboardView },
    { path: '/instances', component: InstancesView },
    { path: '/backups', component: BackupsView },
    { path: '/logs', component: LogsView },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const auth = useAuthStore()
  await auth.check()
  if (!auth.authenticated) {
    return { path: '/login', query: to.path !== '/' ? { redirect: to.fullPath } : undefined }
  }
  return true
})

export default router
