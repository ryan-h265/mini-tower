import { createRouter, createWebHistory } from 'vue-router'
import AppShell from '../components/layout/AppShell.vue'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../pages/LoginPage.vue'),
      meta: { public: true }
    },
    {
      path: '/',
      component: AppShell,
      children: [
        { path: '', redirect: '/home' },
        { path: 'home', name: 'home', component: () => import('../pages/HomePage.vue') },
        { path: 'apps', name: 'apps', component: () => import('../pages/AppsPage.vue') },
        { path: 'apps/:slug', name: 'app-detail', component: () => import('../pages/AppDetailPage.vue') },
        { path: 'runs', name: 'runs', component: () => import('../pages/GlobalRunsPage.vue') },
        { path: 'runs/:runId', name: 'run-detail', component: () => import('../pages/RunDetailPage.vue') },
        { path: 'settings/tokens', name: 'tokens', component: () => import('../pages/TokenSettingsPage.vue') },
        {
          path: 'admin/runners',
          name: 'admin-runners',
          component: () => import('../pages/AdminRunnersPage.vue'),
          meta: { admin: true }
        }
      ]
    }
  ]
})

router.beforeEach((to) => {
  const auth = useAuthStore()

  if (to.meta.public && auth.isAuthenticated) {
    return { path: '/home' }
  }

  if (!to.meta.public && !auth.isAuthenticated) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }

  if (to.meta.admin && !auth.isAdmin) {
    return { path: '/home' }
  }

  return true
})

export default router
