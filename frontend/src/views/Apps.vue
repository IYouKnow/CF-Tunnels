<template>
  <div class="apps-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Registered Apps</h2>
        <p class="subtitle">Manage app identities and API tokens for integrations</p>
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
            <div class="app-icon">{{ app.name.charAt(0).toUpperCase() }}</div>
            <div class="app-meta">
              <div class="app-name">{{ app.name }}</div>
              <div class="app-slug">{{ app.slug }}</div>
            </div>
          </button>
        </div>
      </div>

      <div class="card app-detail-card">
        <div v-if="!selectedApp" class="empty">Select an app to view details and tokens.</div>
        <template v-else>
          <div class="detail-header">
            <div class="detail-heading">
              <div class="detail-title">{{ selectedApp.name }}</div>
              <div class="detail-slug">{{ selectedApp.slug }}</div>
            </div>
            <button class="btn-action danger" @click="deleteSelectedApp">Delete</button>
          </div>
          <div class="detail-body">
            <p class="detail-description">{{ selectedApp.description || 'No description provided' }}</p>

            <div v-if="tokenReveal" class="token-reveal">
              <div class="reveal-header">
                <div class="reveal-heading">Token Created</div>
                <button type="button" class="reveal-close" @click="tokenReveal = null" aria-label="Close"><X :size="16" /></button>
              </div>
              <p class="reveal-sub">Copy this token now. It will only be shown once.</p>
              <div class="reveal-token-row">
                <code class="reveal-token">{{ tokenReveal.token }}<button type="button" class="reveal-copy" :class="{ copied }" @click="copyToken" title="Copy to clipboard"><Copy v-if="!copied" :size="15" /><Check v-else :size="15" /></button></code>
              </div>
            </div>

            <div class="token-section-header">
              <div>
                <div class="section-title">Tokens</div>
                <div class="section-subtitle">Scoped API tokens for this app</div>
              </div>
              <button class="btn-primary" @click="showCreateToken = true">+ New Token</button>
            </div>

            <div v-if="loadingTokens" class="empty">Loading tokens...</div>
            <div v-else-if="tokens.length === 0" class="empty">No tokens created yet.</div>
            <div v-else class="token-list">
              <div v-for="token in tokens" :key="token.id" class="token-item">
                <div class="token-primary">
                  <div class="token-info">
                    <div class="token-name">{{ token.name }}</div>
                    <code class="token-prefix">{{ token.tokenPrefix }}<button type="button" class="prefix-copy" @click="copyPrefix(token.tokenPrefix, token.id)" title="Copy prefix"><Check v-if="copiedPrefix === token.id" :size="12" /><Copy v-else :size="12" /></button></code>
                  </div>
                  <button v-if="!token.revokedAt" class="btn-action danger sm" @click="revokeToken(token.id)">Revoke</button>
                  <button v-else class="btn-action danger sm" @click="deleteToken(token.id)">Delete</button>
                </div>
                <div class="token-secondary">
                  <div class="token-scopes">
                    <span v-if="!token.scopes.length" class="no-scopes">No scopes</span>
                    <span v-for="scope in token.scopes" :key="scope" class="scope-chip">{{ scope }}</span>
                  </div>
                  <div class="token-secondary-end">
                    <span v-if="token.revokedAt" class="badge error">Revoked</span>
                    <span v-else-if="token.expiresAt && new Date(token.expiresAt) < new Date()" class="badge error">Expired</span>
                    <span v-else class="badge running">Active</span>
                    <span class="token-date">Created {{ formatTime(token.createdAt) }}</span>
                    <span class="token-date-sep">·</span>
                    <span class="token-date">Used {{ formatTime(token.lastUsedAt) }}</span>
                  </div>
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
            <h2>Create Token</h2>
            <p class="token-modal-subtitle">Choose the permissions this token should carry.</p>
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
            <p class="scope-footnote">Only supported app permissions can be used here.</p>
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
import { X, Copy, Check } from '@lucide/vue'

export default {
  name: 'Apps',
  components: { X, Copy, Check },
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
    const copied = ref(false)
    const copiedPrefix = ref(null)
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

    const copyPrefix = (prefix, tokenId) => {
      navigator.clipboard.writeText(prefix)
      copiedPrefix.value = tokenId
      setTimeout(() => { copiedPrefix.value = null }, 1500)
    }

    const copyToken = () => {
      if (tokenReveal.value?.token) {
        navigator.clipboard.writeText(tokenReveal.value.token)
        copied.value = true
        setTimeout(() => { copied.value = false }, 1500)
      }
    }

    const revokeToken = async (tokenId) => {
      if (!selectedApp.value) return
      if (!confirm('Revoke this token? It will stop working immediately.')) return
      await api.revokeAppToken(selectedApp.value.id, tokenId)
      await loadTokens(selectedApp.value.id)
    }

    const deleteToken = async (tokenId) => {
      if (!selectedApp.value) return
      if (!confirm('Permanently delete this token? This cannot be undone.')) return
      await api.deleteAppToken(selectedApp.value.id, tokenId)
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
      apps, selectedApp, tokens, loadingApps, loadingTokens,
      showCreateApp, showCreateToken, newApp, newToken,
      appError, tokenError,       tokenReveal, copied, copiedPrefix, scopeOptions,
      formatTime, applyScopePreset, selectApp, copyToken, copyPrefix,
      submitCreateApp, submitCreateToken, revokeToken, deleteToken, deleteSelectedApp
    }
  }
}
</script>

<style scoped>
.apps-page { max-width: 1400px; }

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  gap: 1rem;
}

.header-left h2 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.25rem; }
.subtitle { color: var(--text-secondary); font-size: 0.9rem; }

.apps-grid {
  display: grid;
  grid-template-columns: 280px 1fr;
  gap: 1.5rem;
  align-items: start;
}

.app-list { display: flex; flex-direction: column; }

.app-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  text-align: left;
  padding: 0.75rem 1rem;
  background: transparent;
  border: none;
  border-bottom: 1px solid var(--border);
  color: var(--text-primary);
  cursor: pointer;
  transition: background 0.12s;
}

.app-item:last-child { border-bottom: none; }
.app-item:hover { background: var(--bg-tertiary); }
.app-item.active { background: var(--accent-subtle); }

.app-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: var(--accent-subtle);
  color: var(--accent);
  font-size: 0.8rem;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.app-meta { min-width: 0; }

.app-name { font-size: 0.875rem; font-weight: 500; }
.app-slug { font-size: 0.75rem; color: var(--text-muted); }

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border);
}

.detail-title { font-size: 1rem; font-weight: 600; }
.detail-slug { font-size: 0.8rem; color: var(--text-muted); margin-top: 0.125rem; }

.detail-body { padding: 1.25rem; }
.detail-description { color: var(--text-secondary); font-size: 0.875rem; margin-bottom: 1.5rem; }

.token-section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin-bottom: 1rem;
}

.section-title { font-size: 0.9rem; font-weight: 600; }
.section-subtitle { font-size: 0.8rem; color: var(--text-muted); margin-top: 0.125rem; }

.token-list { display: flex; flex-direction: column; gap: 0.5rem; }

.token-item {
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 0.75rem 1rem;
  background: var(--bg-elevated);
}

.token-primary {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.token-info {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  min-width: 0;
}

.token-name { font-size: 0.875rem; font-weight: 600; white-space: nowrap; }
.token-prefix {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
  font-size: 0.75rem;
  color: var(--text-muted);
  font-family: ui-monospace, monospace;
  position: relative;
}

.prefix-copy {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: 1px solid transparent;
  color: var(--text-muted);
  padding: 0.125rem;
  border-radius: 4px;
  cursor: pointer;
  line-height: 1;
  transition: all 0.12s;
}

.prefix-copy:hover {
  color: var(--text-primary);
  border-color: var(--border);
  background: var(--bg-tertiary);
}



.token-secondary {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.token-scopes {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  min-width: 0;
}

.scope-chip {
  display: inline-flex;
  align-items: center;
  padding: 0.15rem 0.45rem;
  border-radius: 999px;
  background: var(--accent-subtle);
  border: 1px solid rgba(243, 128, 32, 0.15);
  color: var(--accent);
  font-size: 0.7rem;
  font-weight: 500;
  white-space: nowrap;
}

.no-scopes { color: var(--text-muted); font-size: 0.75rem; }

.token-secondary-end {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  flex-shrink: 0;
}

.token-date {
  font-size: 0.7rem;
  color: var(--text-muted);
  white-space: nowrap;
}

.token-date-sep { color: var(--border-light); font-size: 0.7rem; }

.btn-action.sm {
  padding: 0.2rem 0.5rem;
  font-size: 0.7rem;
  line-height: 1.4;
}

.token-reveal {
  margin-bottom: 1.5rem;
  padding: 1rem;
  border: 1px solid var(--accent);
  border-radius: var(--radius-md);
  background: var(--accent-subtle);
}

.reveal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.reveal-heading { font-size: 0.9rem; font-weight: 600; }

.reveal-close {
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 0.25rem;
  border-radius: var(--radius-sm);
  line-height: 1;
}

.reveal-close:hover { color: var(--text-primary); background: var(--bg-tertiary); }

.reveal-sub { font-size: 0.8rem; color: var(--text-secondary); margin-top: 0.25rem; }

.reveal-token-row {
  position: relative;
  margin-top: 0.75rem;
}

.reveal-token {
  display: block;
  padding: 0.75rem 3rem 0.75rem 0.75rem;
  border-radius: var(--radius-sm);
  background: var(--bg-primary);
  font-size: 0.8rem;
  overflow-wrap: anywhere;
  position: relative;
}

.reveal-copy {
  position: absolute;
  top: 50%;
  right: 0.625rem;
  transform: translateY(-50%);
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 0.25rem 0.375rem;
  font-size: 0.875rem;
  cursor: pointer;
  line-height: 1;
  color: #fff;
  transition: background 0.12s;
}

.reveal-copy:hover { background: var(--border); }
.reveal-copy.copied { border-color: var(--success, #4ade80); background: rgba(74, 222, 128, 0.15); }

/* Create Token modal */
.token-modal { max-width: 600px; }

.token-modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1.25rem;
}

.token-modal-header h2 { margin-bottom: 0.25rem; }
.token-modal-subtitle { color: var(--text-secondary); font-size: 0.85rem; }

.token-scope-count {
  padding: 0.35rem 0.75rem;
  border-radius: 999px;
  background: var(--accent-subtle);
  border: 1px solid rgba(243, 128, 32, 0.2);
  color: var(--accent);
  font-size: 0.85rem;
  font-weight: 600;
  white-space: nowrap;
}

.scope-panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  margin-bottom: 0.75rem;
}

.scope-panel-header label { margin-bottom: 0; }

.scope-presets { display: flex; gap: 0.375rem; }

.preset-pill {
  padding: 0.25rem 0.625rem;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.75rem;
  cursor: pointer;
  transition: all 0.12s;
}

.preset-pill:hover {
  border-color: var(--accent);
  color: var(--accent);
  background: var(--accent-subtle);
}

.preset-clear { color: var(--text-muted); }

.selected-scope-strip {
  margin-bottom: 0.75rem;
  padding: 0.75rem 1rem;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: var(--bg-tertiary);
}

.selected-scope-title { font-size: 0.8rem; font-weight: 600; margin-bottom: 0.5rem; }
.selected-scope-empty { color: var(--text-muted); font-size: 0.8rem; }

.scope-selector { display: grid; gap: 0.5rem; }

.scope-option {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  padding: 0.75rem;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  background: transparent;
  cursor: pointer;
  transition: all 0.12s;
}

.scope-option:hover { border-color: var(--border-light); background: var(--bg-tertiary); }

.scope-option.selected {
  border-color: var(--accent);
  background: var(--accent-subtle);
}

.scope-option input {
  width: 16px;
  height: 16px;
  accent-color: var(--accent);
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

.scope-option-copy { min-width: 0; }
.scope-option-copy strong { font-size: 0.85rem; display: block; }
.scope-option-copy small { color: var(--text-muted); font-size: 0.75rem; }

.scope-code {
  font-family: ui-monospace, monospace;
  font-size: 0.7rem;
  padding: 0.15rem 0.4rem;
  border-radius: 999px;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  flex: 0 0 auto;
}

.scope-footnote { margin-top: 0.75rem; color: var(--text-muted); font-size: 0.8rem; }

.error-msg { color: var(--error); margin-top: 0.25rem; font-size: 0.85rem; }

@media (max-width: 960px) {
  .apps-grid { grid-template-columns: 1fr; }
  .detail-header, .token-section-header { flex-direction: column; align-items: flex-start; }
  .token-secondary { flex-direction: column; align-items: flex-start; }
  .token-modal-header, .scope-panel-header, .scope-option-main { flex-direction: column; align-items: flex-start; }
}
</style>
