<template>
  <div class="apps-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Registered Apps</h2>
        <p class="subtitle">Manage app identities and one-time API tokens for future CF Tunnels integrations</p>
      </div>
      <button class="btn-primary" @click="showCreateApp = true">+ New App</button>
    </div>

    <div class="apps-grid">
      <div class="card app-list-card">
        <div class="card-header">Apps</div>
        <div v-if="loadingApps" class="empty">Loading apps...</div>
        <div v-else-if="apps.length === 0" class="empty">No apps registered yet.</div>
        <div v-else class="app-list">
          <button
            v-for="app in apps"
            :key="app.id"
            type="button"
            :class="['app-item', { active: selectedApp && selectedApp.id === app.id }]"
            @click="selectApp(app)"
          >
            <div class="app-name">{{ app.name }}</div>
            <div class="app-slug">{{ app.slug }}</div>
          </button>
        </div>
      </div>

      <div class="card app-detail-card">
        <div v-if="!selectedApp" class="empty">Select an app to view details and tokens.</div>
        <template v-else>
          <div class="card-header detail-header">
            <div>
              <div class="detail-title">{{ selectedApp.name }}</div>
              <div class="detail-slug">{{ selectedApp.slug }}</div>
            </div>
            <button class="btn-secondary" @click="deleteSelectedApp">Delete App</button>
          </div>
          <div class="detail-body">
            <p class="detail-description">{{ selectedApp.description || 'No description provided.' }}</p>

            <div v-if="tokenReveal" class="token-reveal">
              <strong>Copy this token now.</strong>
              <p>This plaintext token will only be shown once.</p>
              <code>{{ tokenReveal.token }}</code>
            </div>

            <div class="token-toolbar">
              <div>
                <div class="section-title">App Tokens</div>
                <div class="section-subtitle">Use a read-only token first while the internal app API is being built out.</div>
              </div>
              <button class="btn-primary" @click="showCreateToken = true">+ New Token</button>
            </div>

            <div v-if="loadingTokens" class="empty">Loading tokens...</div>
            <div v-else-if="tokens.length === 0" class="empty">No tokens created yet.</div>
            <div v-else class="token-list">
              <div v-for="token in tokens" :key="token.id" class="token-item">
                <div class="token-main">
                  <div class="token-name">{{ token.name }}</div>
                  <div class="token-prefix">{{ token.tokenPrefix }}</div>
                </div>
                <div class="token-meta">
                  <div class="token-scopes">{{ token.scopes.join(', ') || 'No scopes' }}</div>
                  <div class="token-status">
                    <span v-if="token.revokedAt" class="badge error">Revoked</span>
                    <span v-else-if="token.expiresAt && new Date(token.expiresAt) < new Date()" class="badge error">Expired</span>
                    <span v-else class="badge running">Active</span>
                  </div>
                </div>
                <div class="token-actions">
                  <small>Created {{ formatTime(token.createdAt) }}</small>
                  <small>Last used {{ formatTime(token.lastUsedAt) }}</small>
                  <button class="btn-action danger" :disabled="!!token.revokedAt" @click="revokeToken(token.id)">Revoke</button>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <div v-if="showCreateApp" class="modal-overlay" @click.self="showCreateApp = false">
      <div class="modal">
        <h2>Create App</h2>
        <form @submit.prevent="submitCreateApp">
          <div class="form-group">
            <label>Name</label>
            <input v-model="newApp.name" type="text" required />
          </div>
          <div class="form-group">
            <label>Slug</label>
            <input v-model="newApp.slug" type="text" placeholder="wireguard-vpn" required />
            <small>Lowercase letters, numbers, and hyphens only.</small>
          </div>
          <div class="form-group">
            <label>Description</label>
            <input v-model="newApp.description" type="text" />
          </div>
          <p v-if="appError" class="error-msg">{{ appError }}</p>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="showCreateApp = false">Cancel</button>
            <button type="submit" class="btn-primary">Create App</button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="showCreateToken && selectedApp" class="modal-overlay" @click.self="showCreateToken = false">
      <div class="modal">
        <h2>Create Token</h2>
        <form @submit.prevent="submitCreateToken">
          <div class="form-group">
            <label>Token Name</label>
            <input v-model="newToken.name" type="text" required />
          </div>
          <div class="form-group">
            <label>Scopes</label>
            <input v-model="newToken.scopes" type="text" placeholder="resources:read" />
            <small>Comma-separated. Example: resources:read</small>
          </div>
          <div class="form-group">
            <label>Expires At</label>
            <input v-model="newToken.expiresAt" type="datetime-local" />
          </div>
          <p v-if="tokenError" class="error-msg">{{ tokenError }}</p>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="showCreateToken = false">Cancel</button>
            <button type="submit" class="btn-primary">Create Token</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import api from '../api'

export default {
  name: 'Apps',
  setup () {
    const apps = ref([])
    const selectedApp = ref(null)
    const tokens = ref([])
    const loadingApps = ref(false)
    const loadingTokens = ref(false)
    const showCreateApp = ref(false)
    const showCreateToken = ref(false)
    const appError = ref('')
    const tokenError = ref('')
    const tokenReveal = ref(null)

    const newApp = ref({
      name: '',
      slug: '',
      description: ''
    })

    const newToken = ref({
      name: '',
      scopes: 'resources:read',
      expiresAt: ''
    })

    const formatTime = (value) => {
      if (!value) return 'never'
      const date = new Date(value)
      if (isNaN(date.getTime())) return 'unknown'
      return date.toLocaleString()
    }

    const loadApps = async () => {
      loadingApps.value = true
      appError.value = ''
      try {
        const items = await api.getApps()
        apps.value = Array.isArray(items) ? items : []
        if (selectedApp.value) {
          const fresh = apps.value.find(app => app.id === selectedApp.value.id)
          selectedApp.value = fresh || null
        }
        if (!selectedApp.value && apps.value.length > 0) {
          await selectApp(apps.value[0])
        }
      } catch (e) {
        apps.value = []
        appError.value = e.response?.data?.error || 'Failed to load apps'
      } finally {
        loadingApps.value = false
      }
    }

    const loadTokens = async (appId) => {
      loadingTokens.value = true
      tokenError.value = ''
      try {
        const items = await api.getAppTokens(appId)
        tokens.value = Array.isArray(items) ? items : []
      } catch (e) {
        tokens.value = []
        tokenError.value = e.response?.data?.error || 'Failed to load tokens'
      } finally {
        loadingTokens.value = false
      }
    }

    const selectApp = async (app) => {
      selectedApp.value = app
      tokenReveal.value = null
      await loadTokens(app.id)
    }

    const submitCreateApp = async () => {
      appError.value = ''
      try {
        const created = await api.createApp(newApp.value)
        showCreateApp.value = false
        newApp.value = { name: '', slug: '', description: '' }
        await loadApps()
        await selectApp(created)
      } catch (e) {
        appError.value = e.response?.data?.error || 'Failed to create app'
      }
    }

    const submitCreateToken = async () => {
      if (!selectedApp.value) return
      tokenError.value = ''
      try {
        const created = await api.createAppToken(selectedApp.value.id, {
          name: newToken.value.name,
          scopes: newToken.value.scopes.split(',').map(scope => scope.trim()).filter(Boolean),
          expiresAt: newToken.value.expiresAt ? new Date(newToken.value.expiresAt).toISOString() : null
        })
        tokenReveal.value = created
        showCreateToken.value = false
        newToken.value = { name: '', scopes: 'resources:read', expiresAt: '' }
        await loadTokens(selectedApp.value.id)
      } catch (e) {
        tokenError.value = e.response?.data?.error || 'Failed to create token'
      }
    }

    const revokeToken = async (tokenId) => {
      if (!selectedApp.value) return
      if (!confirm('Revoke this token? It will stop working immediately.')) return
      await api.revokeAppToken(selectedApp.value.id, tokenId)
      await loadTokens(selectedApp.value.id)
    }

    const deleteSelectedApp = async () => {
      if (!selectedApp.value) return
      if (!confirm(`Delete app "${selectedApp.value.name}"?`)) return
      await api.deleteApp(selectedApp.value.id)
      selectedApp.value = null
      tokens.value = []
      tokenReveal.value = null
      await loadApps()
    }

    onMounted(loadApps)

    return {
      apps,
      selectedApp,
      tokens,
      loadingApps,
      loadingTokens,
      showCreateApp,
      showCreateToken,
      newApp,
      newToken,
      appError,
      tokenError,
      tokenReveal,
      formatTime,
      selectApp,
      submitCreateApp,
      submitCreateToken,
      revokeToken,
      deleteSelectedApp
    }
  }
}
</script>

<style scoped>
.apps-page {
  max-width: 1400px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  gap: 1rem;
}

.header-left h2 {
  font-size: 1.5rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.subtitle {
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.apps-grid {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: 1.5rem;
}

.app-list {
  display: flex;
  flex-direction: column;
}

.app-item {
  text-align: left;
  padding: 1rem 1.25rem;
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--border);
  color: var(--text-primary);
  cursor: pointer;
}

.app-item:hover,
.app-item.active {
  background: var(--bg-tertiary);
}

.app-name,
.detail-title {
  font-weight: 600;
}

.app-slug,
.detail-slug,
.section-subtitle,
.detail-description,
.token-prefix,
.token-actions small {
  color: var(--text-secondary);
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.detail-body {
  padding: 1.25rem;
}

.token-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin: 1.5rem 0 1rem;
}

.section-title {
  font-weight: 600;
}

.token-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.token-item {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 1rem;
  background: var(--bg-tertiary);
}

.token-main,
.token-meta,
.token-actions {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: center;
}

.token-meta,
.token-actions {
  margin-top: 0.5rem;
}

.token-actions {
  flex-wrap: wrap;
}

.token-reveal {
  margin-top: 1rem;
  padding: 1rem;
  border: 1px solid var(--warning);
  background: rgba(245, 158, 11, 0.08);
  border-radius: 10px;
}

.token-reveal code {
  display: block;
  margin-top: 0.75rem;
  padding: 0.75rem;
  border-radius: 8px;
  background: var(--bg-primary);
  overflow-wrap: anywhere;
}

.error-msg {
  color: var(--error);
  margin-top: 0.25rem;
}

@media (max-width: 960px) {
  .apps-grid {
    grid-template-columns: 1fr;
  }

  .token-main,
  .token-meta,
  .token-actions,
  .detail-header,
  .token-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
