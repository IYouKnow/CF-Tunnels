<template>
  <div class="app" :class="theme">
    <div v-if="sidebarOpen" class="sidebar-overlay" @click="sidebarOpen = false" />
    <aside class="sidebar" :class="{ open: sidebarOpen }">
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

        <router-link to="/apps" class="nav-item">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
            <circle cx="9" cy="7" r="4"/>
            <path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
            <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
          </svg>
          <span>Apps</span>
        </router-link>
      </nav>

      <div class="sidebar-footer">
        <div class="account-card">
          <div class="account-header" @click="accountOpen = !accountOpen">
            <div class="account-avatar">{{ userInitial }}</div>
            <div class="account-info">
              <div class="account-name">{{ displayName }}</div>
              <div class="account-role">Admin</div>
            </div>
            <svg :class="['chevron', { open: accountOpen }]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
              <polyline points="6 9 12 15 18 9"/>
            </svg>
          </div>
          <div v-if="accountOpen" class="account-dropdown">
            <button class="account-item" @click="toggleTheme">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
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
              <span>{{ theme === 'dark' ? 'Light mode' : 'Dark mode' }}</span>
            </button>
            <button class="account-item logout" @click="logout">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="16" height="16">
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
                <polyline points="16 17 21 12 16 7"/>
                <line x1="21" y1="12" x2="9" y2="12"/>
              </svg>
              <span>Sign out</span>
            </button>
          </div>
        </div>
        <div class="sidebar-version">
          <div class="vb">
            <div class="vb-top">
              <span class="vl">CF-Tunnels</span>
            </div>
            <span class="vv">{{ appVer || '...' }}</span>
            <div v-if="appUpdate" class="update-available">
              <span class="update-msg">Update available</span>
              <div class="update-actions">
                <button class="ub up" :disabled="updatingApp" @click="doAppUpdate">{{ updatingApp ? '...' : 'Update' }}</button>
              </div>
            </div>
          </div>
          <div class="version-divider"></div>
          <div class="vb">
            <div class="vb-top">
              <span class="vl">cloudflared</span>
            </div>
            <span class="vv">{{ cloudflaredVer || '...' }}</span>
            <div v-if="cloudflaredUpdate" class="update-available">
              <span class="update-msg">Update available</span>
              <div class="update-actions">
                <button class="ub up" :disabled="updating" @click="doUpdate">{{ updating ? '...' : 'Update' }}</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <main class="main-wrapper">
      <header class="topbar">
        <div class="topbar-left">
          <button class="hamburger" @click="sidebarOpen = !sidebarOpen" aria-label="Toggle navigation">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="20" height="20">
              <line x1="3" y1="6" x2="21" y2="6" />
              <line x1="3" y1="12" x2="21" y2="12" />
              <line x1="3" y1="18" x2="21" y2="18" />
            </svg>
          </button>
          <h1 class="page-title">{{ pageTitle }}</h1>
        </div>
        <div class="topbar-actions">
          <slot name="actions"/>
        </div>
      </header>
      <div class="main-content">
        <router-view/>
      </div>
    </main>
  </div>
  <div class="toast-container">
    <div v-for="t in toasts" :key="t.id" :class="['toast', 'toast-' + t.type]">
      {{ t.message }}
    </div>
  </div>
  <ConfirmModal
    :show="dockerUpdateModal"
    title="Update via Docker"
    message="This app is running in Docker. Pull the latest image using your Docker management interface, or run: docker compose pull &amp;&amp; docker compose up -d"
    confirm-text="Got it"
    @confirm="dockerUpdateModal = false"
    @cancel="dockerUpdateModal = false"
  />
</template>

<script>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { currentUser } from '../auth'
import { useToast, showToast } from '../toast'
import { useKeyboardShortcuts } from '../composables/useKeyboardShortcuts'
import ConfirmModal from '../components/ConfirmModal.vue'

const routeTitles = {
  '/': 'Dashboard',
  '/tunnels': 'Tunnels',
  '/dns': 'DNS & Domains',
  '/logs': 'Logs',
  '/apps': 'Registered Apps'
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

    watch(theme, (newTheme) => {
      document.documentElement.setAttribute('data-theme', newTheme)
    })

    watch(() => route.path, () => {
      sidebarOpen.value = false
    })

    const doAppUpdate = async () => {
      if (dockerMode.value) {
        dockerUpdateModal.value = true
        return
      }
      updatingApp.value = true
      try {
        const result = await api.updateApp()
        showToast(result.message || 'Update completed, restarting...')
        appUpdate.value = null
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      } finally {
        updatingApp.value = false
      }
    }

    const doUpdate = async () => {
      updating.value = true
      try {
        const result = await api.updateCloudflared()
        showToast(result.message || 'Update completed')
        cloudflaredUpdate.value = null
      } catch (e) {
        showToast('Update failed: ' + (e.response?.data?.error || e.message), 'error')
      } finally {
        updating.value = false
      }
    }

    const sidebarOpen = ref(false)
    const accountOpen = ref(false)
    const cloudflaredVer = ref('')
    const cloudflaredUpdate = ref(null)
    const appVer = ref('')
    const appUpdate = ref(null)
    const updating = ref(false)
    const updatingApp = ref(false)
    const dockerMode = ref(false)
    const dockerUpdateModal = ref(false)
    const { toasts } = useToast()

    onMounted(async () => {
      document.documentElement.setAttribute('data-theme', theme.value)
      const [cfVer, cfUpdate, aVer, aUpdate] = await Promise.allSettled([
        api.getCloudflaredVersion(),
        api.checkCloudflaredUpdate(),
        api.getAppVersion(),
        api.checkAppUpdate()
      ])
      if (cfVer.status === 'fulfilled') cloudflaredVer.value = cfVer.value.version
      if (cfUpdate.status === 'fulfilled' && cfUpdate.value.hasUpdate) cloudflaredUpdate.value = cfUpdate.value
      if (aVer.status === 'fulfilled') {
        appVer.value = aVer.value.version
        dockerMode.value = aVer.value.docker
      }
      if (aUpdate.status === 'fulfilled' && aUpdate.value.hasUpdate) appUpdate.value = aUpdate.value
    })

    useKeyboardShortcuts({
      'Escape': () => { accountOpen.value = false }
    })

    return { theme, pageTitle, toggleTheme, displayName, userInitial, logout, toasts, sidebarOpen, accountOpen, appVer, cloudflaredVer, cloudflaredUpdate, appUpdate, updating, doUpdate, updatingApp, doAppUpdate, dockerUpdateModal }
  }
}
</script>

<style scoped>
.sidebar-footer {
  padding: 0.5rem;
}

.account-card {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.account-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.625rem;
  cursor: pointer;
  user-select: none;
  transition: background 0.15s;
}

.account-header:hover {
  background: var(--bg-elevated);
}

.account-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: var(--accent);
  color: white;
  font-size: 0.75rem;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.account-info {
  flex: 1;
  min-width: 0;
}

.account-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
  line-height: 1.3;
}

.account-role {
  font-size: 0.7rem;
  color: var(--text-muted);
  line-height: 1.2;
}

.chevron {
  color: var(--text-muted);
  transition: transform 0.2s;
  flex-shrink: 0;
}

.chevron.open {
  transform: rotate(180deg);
}

.account-dropdown {
  border-top: 1px solid var(--border);
  padding: 0.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.account-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.5rem;
  border: none;
  background: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all 0.12s;
  width: 100%;
  text-align: left;
}

.account-item:hover {
  background: var(--bg-elevated);
  color: var(--text-primary);
}

.account-item.logout:hover {
  color: var(--error);
  background: var(--error-subtle);
}

.toast-container {
  position: fixed;
  bottom: 1.5rem;
  right: 1.5rem;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  pointer-events: none;
}

.toast {
  padding: 0.4rem 0.75rem;
  border-radius: 6px;
  font-size: 0.75rem;
  font-weight: 450;
  line-height: 1.4;
  animation: toast-in 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  pointer-events: auto;
  max-width: 320px;
}

.toast-success {
  color: var(--success);
  background: rgba(34, 197, 94, 0.08);
}

.toast-error {
  color: var(--error);
  background: rgba(239, 68, 68, 0.08);
}

.toast-info {
  color: var(--text-secondary);
  background: rgba(160, 160, 160, 0.06);
}

@keyframes toast-in {
  from { transform: translateX(100%); opacity: 0; }
  to { transform: translateX(0); opacity: 1; }
}

.version-divider {
  height: 1px;
  background: var(--border);
  margin: 0.25rem 0;
}

.sidebar-version {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.625rem 1rem;
  border-top: 1px solid var(--border);
}

.vb {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.vb-top {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.vl {
  color: var(--text-muted);
  font-size: 0.65rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.vv {
  color: var(--text-secondary);
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
}

.ub {
  display: inline-flex;
  align-items: center;
  padding: 0.05rem 0.3rem;
  border-radius: 999px;
  font-size: 0.6rem;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
  border: 1px solid rgba(243, 128, 32, 0.2);
  background: var(--accent-subtle);
  color: var(--accent);
}

.ub:hover { background: var(--accent); color: #fff; border-color: var(--accent); }

.ub.up {
  border-color: var(--accent);
  background: var(--accent);
  color: #fff;
  padding: 0.05rem 0.35rem;
  cursor: pointer;
  line-height: 1.3;
}

.ub.up:disabled { opacity: 0.4; cursor: default; }
.ub.up:hover:not(:disabled) { opacity: 0.85; }

.update-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.125rem;
  padding: 0.1rem 0.35rem;
  border-radius: 999px;
  background: var(--accent-subtle);
  border: 1px solid rgba(243, 128, 32, 0.2);
  color: var(--accent);
  font-size: 0.65rem;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
}

.update-badge:hover {
  background: var(--accent);
  color: #fff;
  border-color: var(--accent);
}

.update-btn {
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  border: 1px solid var(--accent);
  background: var(--accent);
  color: #fff;
  font-size: 0.65rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: opacity 0.12s;
}

.update-btn:hover:not(:disabled) { opacity: 0.85; }
.update-btn:disabled { opacity: 0.5; cursor: default; }

.update-available {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  margin-top: 0.15rem;
}

.update-msg {
  color: var(--accent);
  font-size: 0.65rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.update-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.hamburger {
  display: none;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 0.25rem;
  border-radius: var(--radius-sm);
}

.hamburger:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

@media (max-width: 768px) {
  .hamburger {
    display: inline-flex;
  }
  .sidebar {
    transform: translateX(-100%);
    z-index: 200;
    transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1);
  }
  .sidebar.open {
    transform: translateX(0);
  }
}

@media (max-width: 480px) {
  .sidebar {
    width: 260px;
  }
}
</style>
