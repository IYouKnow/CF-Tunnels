<template>
  <div class="dns-page">
    <div class="page-header">
      <div class="header-left">
        <h2>DNS Records</h2>
        <p class="subtitle">View and manage Cloudflare DNS records</p>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" @click="showTutorial = true">API Token Help</button>
        <button class="btn-primary" @click="refresh">Refresh</button>
      </div>
    </div>

    <div v-if="showTutorial" class="modal-overlay" @click.self="showTutorial = false">
      <div class="modal tutorial-modal">
        <div class="modal-header">
          <h2>Creating a Cloudflare API Token</h2>
          <button class="close-btn" @click="showTutorial = false">&times;</button>
        </div>
        <div class="tutorial-content">
          <p class="intro">To manage domains and DNS records, you need a Cloudflare API token with DNS edit permissions.</p>
          <div class="permissions-list">
            <h3>Required Permissions</h3>
            <ul>
              <li><strong>Zone DNS - Edit</strong> - View and modify DNS records</li>
            </ul>
          </div>
          <div class="steps">
            <h3>How to create the token:</h3>
            <ol>
              <li>Go to <strong>Cloudflare Dashboard</strong> → Profile → API Tokens</li>
              <li>Click <strong>Create Custom Token</strong></li>
              <li>Give it a name</li>
              <li>Under <strong>Permissions</strong>, add Zone → DNS → Edit</li>
              <li>Create the token and copy it to <code>.env</code> as <code>CF_API_TOKEN</code></li>
            </ol>
          </div>
        </div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <span>DNS Records</span>
        <div class="controls">
          <select v-model="selectedZone" @change="loadRecords" class="zone-select">
            <option value="">Select a domain</option>
            <option v-for="d in domains" :key="d.id" :value="d.id">{{ d.name }}</option>
          </select>
          <div v-if="totalRecords" class="record-count">{{ totalRecords }} records</div>
        </div>
      </div>
      <div v-if="!selectedZone" class="empty">Select a domain above to view its DNS records</div>
      <div v-else-if="loading" class="empty">Loading records...</div>
      <div v-else-if="records.length === 0" class="empty">No DNS records found for this zone</div>
      <div v-else>
        <div class="table-header">
          <div class="col-type">Type</div>
          <div class="col-name">Name</div>
          <div class="col-content">Content</div>
          <div class="col-ttl">TTL</div>
          <div class="col-tunnel">Tunnel</div>
          <div class="col-actions"></div>
        </div>
        <div v-for="r in records" :key="r.id" class="table-row">
          <div class="col-type"><span class="badge type">{{ r.type }}</span></div>
          <div class="col-name"><code>{{ r.name }}</code></div>
          <div class="col-content"><code>{{ r.content }}</code></div>
          <div class="col-ttl">{{ r.ttl === 1 ? 'Auto' : r.ttl + 's' }}</div>
          <div class="col-tunnel">
            <span v-if="r.tunnel_name" class="tunnel-link" @click="goToTunnel(r.tunnel_name)">{{ r.tunnel_name }}</span>
            <span v-else class="no-domain">-</span>
          </div>
          <div class="col-actions">
            <button class="btn-action danger" @click="deleteRecord(r.zone_id, r.id, r.name)">✕</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { showToast } from '../toast'

export default {
  name: 'DNS',
  setup() {
    const router = useRouter()
    const domains = ref([])
    const records = ref([])
    const selectedZone = ref('')
    const loading = ref(false)
    const totalRecords = ref(0)
    const showTutorial = ref(false)

    const loadDomains = async () => {
      try {
        const result = await api.getDomains(1, 100)
        domains.value = result.domains || []
        if (domains.value.length > 0 && !selectedZone.value) {
          selectedZone.value = domains.value[0].id
          loadRecords()
        }
      } catch (e) {
        showToast('Failed to load domains: ' + (e.response?.data?.error || e.message), 'error')
      }
    }

    const loadRecords = async () => {
      if (!selectedZone.value) return
      loading.value = true
      try {
        const result = await api.getDNSRecords(selectedZone.value)
        records.value = result.records || []
        totalRecords.value = result.total || 0
      } catch (e) {
        showToast('Failed to load records: ' + (e.response?.data?.error || e.message), 'error')
      }
      loading.value = false
    }

    const deleteRecord = async (zoneId, recordId, name) => {
      if (!confirm(`Delete DNS record "${name}"?`)) return
      try {
        await api.deleteDNSRecord(zoneId, recordId)
        showToast('DNS record deleted')
        loadRecords()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
    }

    const refresh = () => {
      loadDomains()
      if (selectedZone.value) loadRecords()
    }

    const goToTunnel = (tunnelName) => {
      router.push('/tunnels')
    }

    onMounted(loadDomains)

    return { domains, records, selectedZone, loading, totalRecords, showTutorial, loadRecords, deleteRecord, refresh, goToTunnel }
  }
}
</script>

<style scoped>
.dns-page { max-width: 1200px; }

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
}

.header-left h2 { font-size: 1.5rem; font-weight: 600; margin-bottom: 0.25rem; }
.subtitle { color: var(--text-secondary); font-size: 0.9rem; }
.header-actions { display: flex; gap: 0.75rem; }
.btn-primary, .btn-secondary { display: flex; align-items: center; gap: 0.5rem; }

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.controls { display: flex; align-items: center; gap: 1rem; }

.zone-select {
  padding: 0.5rem 0.75rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-primary);
  font-size: 0.9rem;
  min-width: 220px;
}

.record-count { font-size: 0.85rem; color: var(--text-secondary); }

.table-header, .table-row {
  display: grid;
  grid-template-columns: 70px 2fr 2.5fr 80px 1.5fr 40px;
  gap: 1rem;
  padding: 0.75rem 1.25rem;
  align-items: center;
}

.table-header {
  background: var(--bg-tertiary);
  font-weight: 500;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.table-row {
  border-bottom: 1px solid var(--border);
  font-size: 0.875rem;
  transition: background 0.15s;
}

.table-row:last-child { border-bottom: none; }
.table-row:hover { background: var(--bg-tertiary); }

.col-type { text-align: center; }

.badge.type {
  font-size: 0.7rem;
  padding: 0.15rem 0.5rem;
  background: rgba(243, 128, 32, 0.1);
  color: var(--accent);
  border-radius: 4px;
  font-weight: 600;
  letter-spacing: 0.5px;
}

.col-name code, .col-content code {
  font-size: 0.8rem;
  color: var(--text-primary);
  background: var(--bg-tertiary);
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  word-break: break-all;
}

.col-ttl { font-size: 0.85rem; color: var(--text-secondary); }

.tunnel-link {
  color: var(--accent);
  cursor: pointer;
  font-size: 0.85rem;
  font-weight: 500;
}

.tunnel-link:hover { text-decoration: underline; }

.no-domain { color: var(--text-secondary); }

.btn-action.danger {
  background: none;
  border: none;
  color: var(--text-secondary);
  font-size: 1rem;
  cursor: pointer;
  padding: 0.25rem;
  border-radius: 4px;
  transition: all 0.15s;
}

.btn-action.danger:hover {
  color: var(--error);
  background: rgba(239, 68, 68, 0.1);
}

.empty {
  padding: 2.5rem;
  text-align: center;
  color: var(--text-secondary);
  font-size: 0.95rem;
}

/* Modal styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.modal {
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 1.5rem;
  width: 100%;
  max-width: 520px;
  max-height: 85vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.modal-header h2 { font-size: 1.25rem; font-weight: 600; }

.close-btn {
  background: none;
  border: none;
  font-size: 1.5rem;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 0.25rem;
  line-height: 1;
}

.close-btn:hover { color: var(--text-primary); }

.tutorial-content .intro { margin-bottom: 1rem; color: var(--text-secondary); }
.permissions-list { background: var(--bg-tertiary); border-radius: 8px; padding: 1rem; margin-bottom: 1rem; border: 1px solid var(--border); }
.permissions-list h3 { font-size: 0.95rem; margin-bottom: 0.75rem; }
.permissions-list ul { list-style: none; padding: 0; }
.permissions-list li { padding: 0.3rem 0; font-size: 0.9rem; }
.steps { margin-bottom: 1rem; }
.steps h3 { font-size: 0.95rem; margin-bottom: 0.75rem; }
.steps ol { padding-left: 1.25rem; }
.steps li { margin-bottom: 0.5rem; font-size: 0.9rem; line-height: 1.5; }

@media (max-width: 768px) {
  .table-header, .table-row {
    grid-template-columns: 60px 1.5fr 2fr 60px;
  }
  .col-ttl, .col-tunnel { display: none; }
}
</style>
