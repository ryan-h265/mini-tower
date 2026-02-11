import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { VueQueryPlugin, QueryClient } from '@tanstack/vue-query'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import { setUnauthorizedHandler } from './api/client'
import { useTheme } from './composables/useTheme'
import './assets/variables.css'
import './assets/global.css'

const app = createApp(App)
const pinia = createPinia()
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1
    }
  }
})

app.use(pinia)
app.use(VueQueryPlugin, { queryClient })

const auth = useAuthStore(pinia)
useTheme()

setUnauthorizedHandler(() => {
  auth.logout()
  if (router.currentRoute.value.path !== '/login') {
    void router.replace('/login')
  }
})

void auth.rehydrate().finally(() => {
  app.use(router)
  app.mount('#app')
})
