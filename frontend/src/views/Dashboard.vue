<template>
  <div class="dashboard-page">
  <div class="page-header">
    <div class="header-left">
        <h2>Dashboard</h2>
        <p class="subtitle">Overview of your Cloudflare Tunnels</p>
      </div>
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon total">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 12h-4l-3 9L9 3l-3 9H2"/>
          </svg>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ status.total }}</div>
          <div class="stat-label">Total Tunnels</div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon running">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
            <polyline points="22 4 12 14.01 9 11.01"/>
          </svg>
        </div>
        <div class="stat-content">
          <div class="stat-value" style="color: var(--success)">{{ status.running }}</div>
          <div class="stat-label">Running</div>
        </div>
      </div>
      
      <div class="stat-card">
        <div class="stat-icon stopped">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
          </svg>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ status.stopped }}</div>
          <div class="stat-label">Stopped</div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">Recent Tunnels</div>
      <div class="tunnel-list" v-if="tunnels.length > 0">
        <div v-for="tunnel in tunnels" :key="tunnel.id" class="tunnel-item">
          <div class="tunnel-info">
            <div class="tunnel-name">{{ tunnel.name }}</div>
            <div class="tunnel-uuid">{{ tunnel.uuid || 'No UUID' }}</div>
          </div>
          <div class="tunnel-meta">
            <span v-if="tunnel.zone_id && tunnel.subdomain" class="tunnel-domain">
              {{ tunnel.subdomain }}.{{ getDomainName(tunnel.zone_id) }}
            </span>
            <span :class="['badge', tunnel.status]">{{ tunnel.status }}</span>
          </div>
        </div>
      </div>
      <div v-else class="empty">
        <p>No tunnels yet. Create your first tunnel to get started.</p>
        <router-link to="/tunnels" class="btn-primary" style="margin-top: 1rem; display: inline-flex;">
          Go to Tunnels
        </router-link>
      </div>
    </div>

    <div class="card" style="margin-top: 1.5rem;">
      <div class="card-header">Quick Actions</div>
      <div class="quick-actions">
        <router-link to="/tunnels" class="action-card">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="12" y1="5" x2="12" y2="19"/>
            <line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          <span>Create Tunnel</span>
        </router-link>
        <router-link to="/dns" class="action-card">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="10"/>
            <line x1="2" y1="12" x2="22" y2="12"/>
            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
          </svg>
          <span>Manage DNS</span>
        </router-link>
        <router-link to="/logs" class="action-card">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
            <polyline points="14 2 14 8 20 8"/>
            <line x1="16" y1="13" x2="8" y2="13"/>
            <line x1="16" y1="17" x2="8" y2="17"/>
          </svg>
          <span>View Logs</span>
        </router-link>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted } from 'vue'
import api from '../api'

export default {
  name: 'Dashboard',
  setup() {
    const status = ref({ total: 0, running: 0, stopped: 0 })
    const tunnels = ref([])
    const domains = ref([])

    const getDomainName = (zoneId) => {
      const domain = domains.value.find(d => d.id === zoneId)
      return domain ? domain.name : ''
    }

    const loadData = async () => {
      try {
        status.value = await api.getStatus()
        const tunnelData = await api.getTunnels()
        tunnels.value = (tunnelData || []).slice(0, 5)
        const domainData = await api.getDomains()
        domains.value = domainData?.domains || []
      } catch (e) {
        console.error(e)
        tunnels.value = []
        domains.value = []
      }
    }

    onMounted(loadData)
    return { status, tunnels, domains, getDomainName }
  }
}
</script>

<style scoped>
.dashboard-page {
  max-width: 1200px;
}

.page-header {
  margin-bottom: 1.5rem;
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

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.stat-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.25rem;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.stat-icon svg {
  width: 24px;
  height: 24px;
}

.stat-icon.total {
  background: rgba(243, 128, 32, 0.15);
  color: var(--accent);
}

.stat-icon.running {
  background: rgba(34, 197, 94, 0.15);
  color: var(--success);
}

.stat-icon.stopped {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.stat-content {
  flex: 1;
}

.stat-value {
  font-size: 2rem;
  font-weight: 700;
  line-height: 1.2;
}

.stat-label {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin-top: 0.125rem;
}

.tunnel-list {
  display: flex;
  flex-direction: column;
}

.tunnel-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border);
  transition: background 0.2s;
}

.tunnel-item:last-child {
  border-bottom: none;
}

.tunnel-item:hover {
  background: var(--bg-tertiary);
}

.tunnel-name {
  font-weight: 500;
  font-size: 0.95rem;
}

.tunnel-uuid {
  font-size: 0.75rem;
  color: var(--text-secondary);
  font-family: monospace;
  margin-top: 0.25rem;
}

.tunnel-meta {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.tunnel-domain {
  font-size: 0.8rem;
  color: var(--accent);
}

.quick-actions {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  padding: 1rem 1.25rem;
}

.action-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  padding: 1.5rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 8px;
  text-decoration: none;
  color: var(--text-primary);
  transition: all 0.2s;
}

.action-card:hover {
  border-color: var(--accent);
  background: rgba(243, 128, 32, 0.05);
}

.action-card svg {
  width: 28px;
  height: 28px;
  color: var(--accent);
}

.action-card span {
  font-size: 0.9rem;
  font-weight: 500;
}

@media (max-width: 768px) {
  .stats-grid {
    grid-template-columns: 1fr;
  }
  .quick-actions {
    grid-template-columns: 1fr;
  }
}
</style>
