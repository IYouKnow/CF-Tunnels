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
                  <div class="token-scopes">
                    <span v-if="!token.scopes.length" class="scope-chip empty">No scopes</span>
                    <span v-for="scope in token.scopes" :key="scope" class="scope-chip">{{ scope }}</span>
                  </div>
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
      <div class="modal token-modal">
        <div class="token-modal-header">
          <div>
            <p class="eyebrow">Scoped access</p>
            <h2>Create Token</h2>
            <p class="token-modal-subtitle">Choose the exact permissions this token should carry. Only supported app scopes can be selected.</p>
          </div>
          <div class="token-scope-count">{{ newToken.scopes.length }} selected</div>
        </div>
        <form @submit.prevent="submitCreateToken">
          <div class="form-group">
            <label>Token Name</label>
            <input v-model="newToken.name" type="text" placeholder="Default token" required />
          </div>
          <div class="form-group token-scope-panel">
            <div class="scope-panel-header">
              <label>Permissions</label>
              <div class="scope-presets">
                <button type="button" class="preset-pill" @click="applyScopePreset(['resources:read', 'dns:read'])">Read only</button>
                <button type="button" class="preset-pill" @click="applyScopePreset(['resources:read', 'dns:read', 'dns:create', 'dns:update'])">DNS manager</button>
                <button type="button" class="preset-pill preset-clear" @click="applyScopePreset([])">Clear</button>
              </div>
            </div>
            <div class="selected-scope-strip">
              <div class="selected-scope-title">Selected scopes</div>
              <div v-if="newToken.scopes.length" class="token-scopes">
                <span v-for="scope in newToken.scopes" :key="scope" class="scope-chip">{{ scope }}</span>
              </div>
              <p v-else class="selected-scope-empty">No permissions selected yet.</p>
            </div>
            <div class="scope-selector">
              <label v-for="scope in scopeOptions" :key="scope.value" :class="['scope-option', { selected: newToken.scopes.includes(scope.value) }]">
                <input v-model="newToken.scopes" type="checkbox" :value="scope.value" />
                <div class="scope-option-main">
                  <div class="scope-option-copy">
                    <strong>{{ scope.label }}</strong>
                    <small>{{ scope.description }}</small>
                  </div>
                  <span class="scope-code">{{ scope.value }}</span>
                </div>
              </label>
            </div>
            <p class="scope-footnote">Only supported app permissions can be used here. Tunnel scopes are intentionally excluded for now.</p>
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
import { APP_SCOPE_OPTIONS, APP_SCOPE_PRESETS } from '../constants/appScopes'

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
    const scopeOptions = APP_SCOPE_OPTIONS

    const newApp = ref({
      name: '',
      slug: '',
      description: ''
    })

    const newToken = ref({
      name: '',
      scopes: [...APP_SCOPE_PRESETS.readOnly],
      expiresAt: ''
    })

    const formatTime = (value) => {
      if (!value) return 'never'
      const date = new Date(value)
      if (isNaN(date.getTime())) return 'unknown'
      return date.toLocaleString()
    }

    const applyScopePreset = (preset) => {
      newToken.value.scopes = [...preset]
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
          scopes: [...newToken.value.scopes],
          expiresAt: newToken.value.expiresAt ? new Date(newToken.value.expiresAt).toISOString() : null
        })
        tokenReveal.value = created
        showCreateToken.value = false
        newToken.value = { name: '', scopes: [...APP_SCOPE_PRESETS.readOnly], expiresAt: '' }
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
      scopeOptions,
      formatTime,
      applyScopePreset,
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

.token-scopes {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.scope-chip {
  display: inline-flex;
  align-items: center;
  padding: 0.28rem 0.65rem;
  border-radius: 999px;
  background: rgba(59, 130, 246, 0.16);
  border: 1px solid rgba(59, 130, 246, 0.28);
  color: #dbeafe;
  font-size: 0.82rem;
}

.scope-chip.empty {
  background: transparent;
  border-color: var(--border);
  color: var(--text-secondary);
}

.scope-presets {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.preset-pill {
  padding: 0.45rem 0.8rem;
  border: 1px solid rgba(148, 163, 184, 0.24);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: var(--text-primary);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
}

.preset-pill:hover {
  border-color: rgba(45, 212, 191, 0.4);
  background: rgba(45, 212, 191, 0.08);
}

.preset-clear {
  color: var(--text-secondary);
}

.scope-selector {
  display: grid;
  gap: 0.75rem;
}

.scope-option {
  display: flex;
  gap: 0.85rem;
  align-items: center;
  padding: 1rem;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
  cursor: pointer;
  transition: border-color 0.18s ease, background 0.18s ease, box-shadow 0.18s ease;
}

.scope-option:hover {
  border-color: rgba(59, 130, 246, 0.32);
  background: rgba(59, 130, 246, 0.05);
}

.scope-option.selected {
  border-color: rgba(45, 212, 191, 0.45);
  background: rgba(45, 212, 191, 0.08);
  box-shadow: inset 0 0 0 1px rgba(45, 212, 191, 0.12);
}

.scope-option input {
  width: 16px;
  height: 16px;
  accent-color: #14b8a6;
  flex: 0 0 auto;
}

.scope-option-main {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  min-width: 0;
  width: 100%;
}

.scope-option-copy {
  display: grid;
  gap: 0.3rem;
  min-width: 0;
}

.scope-code,
.scope-option-copy small {
  color: var(--text-secondary);
}

.scope-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.78rem;
  padding: 0.22rem 0.45rem;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  flex: 0 0 auto;
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

.token-modal {
  width: min(900px, 96vw);
  max-width: min(900px, 96vw);
  overflow-x: hidden;
  scrollbar-width: thin;
  scrollbar-color: rgba(243, 128, 32, 0.7) rgba(255, 255, 255, 0.05);
}

.token-modal::-webkit-scrollbar {
  width: 12px;
}

.token-modal::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.04);
  border-radius: 999px;
}

.token-modal::-webkit-scrollbar-thumb {
  background: linear-gradient(180deg, rgba(243, 128, 32, 0.9), rgba(255, 157, 77, 0.85));
  border-radius: 999px;
  border: 2px solid transparent;
  background-clip: padding-box;
}

.token-modal::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(180deg, rgba(255, 157, 77, 0.95), rgba(243, 128, 32, 0.95));
  border: 2px solid transparent;
  background-clip: padding-box;
}

.token-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1.2rem;
}

.eyebrow {
  margin: 0 0 0.35rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-size: 0.72rem;
  font-weight: 700;
  color: #0f766e;
}

.token-modal-subtitle {
  margin: 0.35rem 0 0;
  color: var(--text-secondary);
  max-width: 52ch;
}

.token-scope-count {
  padding: 0.5rem 0.85rem;
  border-radius: 999px;
  background: rgba(45, 212, 191, 0.08);
  border: 1px solid rgba(45, 212, 191, 0.18);
  color: #99f6e4;
  font-size: 0.88rem;
  font-weight: 600;
}

.token-scope-panel {
  margin-bottom: 0;
}

.scope-panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin-bottom: 0.85rem;
}

.selected-scope-strip {
  margin-bottom: 0.9rem;
  padding: 0.9rem 1rem;
  border: 1px solid rgba(148, 163, 184, 0.16);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.02);
}

.selected-scope-title {
  margin-bottom: 0.55rem;
  font-size: 0.88rem;
  font-weight: 600;
}

.selected-scope-empty {
  margin: 0;
  color: var(--text-secondary);
  font-size: 0.9rem;
}

.scope-footnote {
  margin: 0.85rem 0 0;
  color: var(--text-secondary);
  font-size: 0.84rem;
  line-height: 1.45;
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

  .token-modal-header,
  .scope-panel-header,
  .scope-option-main {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
