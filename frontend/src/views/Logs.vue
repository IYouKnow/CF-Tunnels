<template>
  <div class="logs-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Logs</h2>
        <p class="subtitle">View tunnel activity and errors</p>
      </div>
      <div class="header-actions">
        <select v-model="selectedTunnel" class="tunnel-select" @change="loadLogs">
          <option value="">All Tunnels</option>
          <option v-for="t in tunnels" :key="t.id" :value="t.id">{{ t.name }}</option>
        </select>
        <button class="btn-primary" @click="loadLogs">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18">
            <polyline points="23 4 23 10 17 10"/>
            <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
          </svg>
          Refresh
        </button>
      </div>
    </div>

    <div class="card">
      <div v-if="loading" class="empty">
        <p>Loading logs…</p>
      </div>
      <div v-else-if="loadError" class="empty error-state">
        <p>{{ loadError }}</p>
        <p class="hint">Sign out and sign in again, or open the app from the same URL as the API (Vite proxy or the Go server).</p>
      </div>
      <div class="log-list" v-else-if="logs.length">
        <div class="log-header">
          <div class="log-col-time">Time</div>
          <div class="log-col-tunnel">Tunnel</div>
          <div class="log-col-level">Level</div>
          <div class="log-col-message">Message</div>
        </div>
        <div v-for="log in logs" :key="log.id" :class="['log-entry', log.level]">
          <div class="log-col-time">{{ formatTime(log.timestamp) }}</div>
          <div class="log-col-tunnel">{{ getTunnelName(log.tunnel_id) }}</div>
          <div class="log-col-level">
            <span :class="['level-badge', log.level]">{{ log.level }}</span>
          </div>
          <div class="log-col-message">{{ log.message }}</div>
        </div>
      </div>
      <div v-else class="empty">
        <p>No logs found.</p>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted, onUnmounted } from 'vue'
import api from '../api'

export default {
  name: 'Logs',
  setup() {
    const logs = ref([])
    const tunnels = ref([])
    const selectedTunnel = ref('')
    const loading = ref(true)
    const loadError = ref('')
    let refreshInterval = null

    const normalizeLogList = (result) => {
      if (result == null) return []
      return Array.isArray(result) ? result : []
    }

    const getTunnelName = (id) => {
      if (id == null || id === '') return '-'
      if (!Array.isArray(tunnels.value)) return 'Unknown'
      const n = Number(id)
      const t = tunnels.value.find(t => t.id === id || t.id === n)
      return t ? t.name : 'Unknown'
    }

    const loadLogs = async () => {
      loadError.value = ''
      try {
        if (selectedTunnel.value) {
          const result = await api.getTunnelLogs(selectedTunnel.value)
          logs.value = normalizeLogList(result)
        } else {
          const result = await api.getAllLogs()
          logs.value = normalizeLogList(result)
        }
      } catch (e) {
        console.error(e)
        const msg = e.response?.data?.error || e.message || 'Failed to load logs'
        loadError.value = typeof msg === 'string' ? msg : 'Failed to load logs'
        logs.value = []
      } finally {
        loading.value = false
      }
    }

    const loadTunnels = async () => {
      try {
        tunnels.value = await api.getTunnels()
      } catch (e) {
        console.error(e)
      }
    }

    const formatTime = (timestamp) => {
      if (!timestamp) return '-'
      const date = new Date(timestamp)
      if (isNaN(date.getTime())) return '-'
      return date.toLocaleString()
    }

    onMounted(async () => {
      await loadTunnels()
      await loadLogs()
      refreshInterval = setInterval(loadLogs, 5000)
    })

    onUnmounted(() => {
      if (refreshInterval) {
        clearInterval(refreshInterval)
      }
    })

    return { logs, tunnels, selectedTunnel, loadLogs, getTunnelName, formatTime, loading, loadError }
  }
}
</script>

<style scoped>
.logs-page {
  max-width: 1400px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
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

.header-actions {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}

.tunnel-select {
  padding: 0.5rem 1rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-primary);
  font-size: 0.9rem;
}

.btn-primary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.log-list {
  display: flex;
  flex-direction: column;
}

.log-header {
  display: grid;
  grid-template-columns: 180px 150px 80px 1fr;
  gap: 1rem;
  padding: 0.875rem 1.25rem;
  background: var(--bg-tertiary);
  font-weight: 500;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.log-entry {
  display: grid;
  grid-template-columns: 180px 150px 80px 1fr;
  gap: 1rem;
  padding: 0.875rem 1.25rem;
  border-bottom: 1px solid var(--border);
  font-size: 0.9rem;
  transition: background 0.2s;
}

.log-entry:hover {
  background: var(--bg-tertiary);
}

.log-entry:last-child {
  border-bottom: none;
}

.log-col-time {
  color: var(--text-secondary);
  font-size: 0.85rem;
}

.log-col-tunnel {
  font-weight: 500;
}

.level-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 500;
  text-transform: uppercase;
}

.level-badge.info {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success);
}

.level-badge.error {
  background: rgba(239, 68, 68, 0.15);
  color: var(--error);
}

.level-badge.warning {
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning);
}

.log-col-message {
  font-family: monospace;
  font-size: 0.85rem;
  word-break: break-all;
}

.empty {
  padding: 3rem;
  text-align: center;
  color: var(--text-secondary);
}

.empty.error-state {
  color: var(--error);
}

.empty .hint {
  margin-top: 0.75rem;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

@media (max-width: 900px) {
  .log-header,
  .log-entry {
    grid-template-columns: 1fr 1fr;
  }
  .log-col-time,
  .log-col-level {
    display: none;
  }
}
</style>