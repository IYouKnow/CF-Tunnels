import { createRouter, createWebHashHistory } from 'vue-router'
import Dashboard from './views/Dashboard.vue'
import Tunnels from './views/Tunnels.vue'
import DNS from './views/DNS.vue'
import Logs from './views/Logs.vue'

const routes = [
  { path: '/', name: 'Dashboard', component: Dashboard },
  { path: '/tunnels', name: 'Tunnels', component: Tunnels },
  { path: '/dns', name: 'DNS', component: DNS },
  { path: '/logs', name: 'Logs', component: Logs }
]

export default createRouter({
  history: createWebHashHistory(),
  routes
})