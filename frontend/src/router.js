import { createRouter, createWebHashHistory } from 'vue-router'
import MainLayout from './layouts/MainLayout.vue'
import Dashboard from './views/Dashboard.vue'
import Tunnels from './views/Tunnels.vue'
import DNS from './views/DNS.vue'
import Logs from './views/Logs.vue'
import Login from './views/Login.vue'
import { refreshAuth } from './auth'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { public: true }
  },
  {
    path: '/',
    component: MainLayout,
    meta: { requiresAuth: true },
    children: [
      { path: '', name: 'Dashboard', component: Dashboard },
      { path: 'tunnels', name: 'Tunnels', component: Tunnels },
      { path: 'dns', name: 'DNS', component: DNS },
      { path: 'logs', name: 'Logs', component: Logs }
    ]
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

router.beforeEach(async (to) => {
  if (to.meta.public) {
    const ok = await refreshAuth()
    if (to.path === '/login' && ok) {
      return { path: '/', replace: true }
    }
    return true
  }
  const ok = await refreshAuth()
  if (ok) return true
  return { path: '/login', query: { redirect: to.fullPath } }
})

export default router
