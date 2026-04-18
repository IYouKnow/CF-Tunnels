<template>
  <div class="app" :class="theme">
    <aside class="sidebar">
      <div class="sidebar-header">
        <div class="brand">
          <span>CF Tunnels</span>
        </div>
      </div>

      <nav class="sidebar-nav">
        <router-link to="/" class="nav-item">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
            <polyline points="9 22 9 12 15 12 15 22"/>
          </svg>
          <span>Dashboard</span>
        </router-link>

        <router-link to="/tunnels" class="nav-item">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 12h-4l-3 9L9 3l-3 9H2"/>
          </svg>
          <span>Tunnels</span>
        </router-link>

        <router-link to="/dns" class="nav-item">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="2" y1="12" x2="22" y2="12"/>
            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
          </svg>
          <span>DNS</span>
        </router-link>

        <router-link to="/logs" class="nav-item">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
            <polyline points="10 9 9 9 8 9"/>
          </svg>
          <span>Logs</span>
        </router-link>
      </nav>

      <div class="sidebar-footer">
        <button class="theme-toggle" @click="toggleTheme">
          <svg v-if="theme === 'dark'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="5"/>
            <line x1="12" y1="1" x2="12" y2="3"/>
            <line x1="12" y1="21" x2="12" y2="23"/>
            <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/>
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
            <line x1="1" y1="12" x2="3" y2="12"/>
            <line x1="21" y1="12" x2="23" y2="12"/>
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
          </svg>
          <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          <span>{{ theme === 'dark' ? 'Light Mode' : 'Dark Mode' }}</span>
        </button>

        <div class="user-info">
          <div class="user-avatar">{{ userInitial }}</div>
          <span class="user-name">{{ displayName }}</span>
        </div>
        <button type="button" class="btn-secondary logout-btn" @click="logout">
          Sign out
        </button>
      </div>
    </aside>

    <main class="main-wrapper">
      <header class="topbar">
        <h1 class="page-title">{{ pageTitle }}</h1>
        <div class="topbar-actions">
          <slot name="actions"/>
        </div>
      </header>
      <div class="main-content">
        <router-view/>
      </div>
    </main>
  </div>
</template>

<script>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { currentUser } from '../auth'

const routeTitles = {
  '/': 'Dashboard',
  '/tunnels': 'Tunnels',
  '/dns': 'DNS & Domains',
  '/logs': 'Logs'
}

export default {
  name: 'MainLayout',
  setup () {
    const theme = ref(localStorage.getItem('theme') || 'dark')
    const route = useRoute()
    const router = useRouter()

    const pageTitle = computed(() => routeTitles[route.path] || 'Dashboard')

    const displayName = computed(() => currentUser.value || 'Admin')
    const userInitial = computed(() => {
      const n = displayName.value
      return n ? String(n).charAt(0).toUpperCase() : 'A'
    })

    const toggleTheme = () => {
      theme.value = theme.value === 'dark' ? 'light' : 'dark'
      localStorage.setItem('theme', theme.value)
    }

    const logout = async () => {
      try {
        await api.logout()
      } finally {
        currentUser.value = null
        router.replace('/login')
      }
    }

    onMounted(() => {
      document.documentElement.setAttribute('data-theme', theme.value)
    })

    watch(theme, (newTheme) => {
      document.documentElement.setAttribute('data-theme', newTheme)
    })

    return { theme, pageTitle, toggleTheme, displayName, userInitial, logout }
  }
}
</script>

<style scoped>
.logout-btn {
  width: 100%;
  font-size: 0.85rem;
}
</style>
