<template>
  <div class="dns-page">
    <div class="page-header">
      <div class="header-left">
        <h2>DNS Records</h2>
        <p class="subtitle">View and manage Cloudflare DNS records</p>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" @click="showTutorial = true">API Token Help</button>
        <button class="btn-primary" @click="openCreateModal">Create Record</button>
        <button class="btn-secondary" @click="refresh">Refresh</button>
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

    <div v-if="showForm" class="modal-overlay" @mousedown.self="closeForm">
      <div class="modal">
        <div class="modal-header">
          <h2>{{ editingRecord ? 'Edit DNS Record' : 'Create DNS Record' }}</h2>
          <button class="close-btn" @click="closeForm">&times;</button>
        </div>
        <form @submit.prevent="saveRecord">
          <div class="form-group">
            <label>Type</label>
            <select v-model="form.type" required>
              <option value="A">A</option>
              <option value="AAAA">AAAA</option>
              <option value="CNAME">CNAME</option>
              <option value="TXT">TXT</option>
              <option value="MX">MX</option>
              <option value="NS">NS</option>
              <option value="SRV">SRV</option>
            </select>
          </div>
          <div class="form-group">
            <label>Name</label>
            <input v-model="form.name" type="text" placeholder="e.g. www or @ for root" required />
            <small>Subdomain part only (the zone domain is appended automatically)</small>
          </div>
          <div class="form-group">
            <label>Content</label>
            <input v-model="form.content" type="text" placeholder="e.g. 192.0.2.1 or target.example.com" required />
          </div>
          <div class="form-group" v-if="form.type !== 'TXT'">
            <label class="toggle-label">
              <span class="toggle-track">
                <input v-model="form.proxied" type="checkbox" />
                <span class="toggle-thumb"></span>
              </span>
              <span class="toggle-text">{{ form.proxied ? 'Proxied' : 'DNS Only' }}</span>
            </label>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="closeForm">Cancel</button>
            <button type="submit" class="btn-primary" :disabled="saving">
              {{ saving ? 'Saving...' : editingRecord ? 'Update' : 'Create' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <ConfirmModal
      :show="showDeleteConfirm"
      title="Delete DNS Record"
      :message="deletingRecord ? `Are you sure you want to delete the DNS record &quot;${deletingRecord.name}&quot;?` : ''"
      confirm-text="Delete"
      danger
      :loading="saving"
      loading-text="Deleting..."
      @confirm="confirmDelete"
      @cancel="showDeleteConfirm = false"
    />

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
          <div class="col-proxy">Proxy</div>
          <div class="col-ttl">TTL</div>
          <div class="col-tunnel">Tunnel</div>
          <div class="col-actions">Actions</div>
        </div>
        <div v-for="r in records" :key="r.id" class="table-row">
          <div class="col-type"><span class="badge type">{{ r.type }}</span></div>
          <div class="col-name"><code>{{ r.name }}</code></div>
          <div class="col-content"><code>{{ r.content }}</code></div>
          <div class="col-proxy">
            <span v-if="r.proxied" class="badge proxied" title="Proxied (orange cloud)">&#9679; Proxied</span>
            <span v-else class="badge unproxied" title="DNS only (gray cloud)">&#9675; DNS Only</span>
          </div>
          <div class="col-ttl">{{ r.ttl === 1 ? 'Auto' : r.ttl + 's' }}</div>
          <div class="col-tunnel">
            <span v-if="r.tunnel_name" class="tunnel-link" @click="goToTunnel(r.tunnel_name)">{{ r.tunnel_name }}</span>
            <span v-else class="no-domain">-</span>
          </div>
          <div class="col-actions">
            <button class="btn-icon" @click="openEditModal(r)" title="Edit"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4Z"/></svg></button>
            <button class="btn-icon danger" @click="deleteRecord(r.zone_id, r.id, r.name)" title="Delete"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/></svg></button>
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
import ConfirmModal from '../components/ConfirmModal.vue'

export default {
  name: 'DNS',
  components: { ConfirmModal },
  setup() {
    const router = useRouter()
    const domains = ref([])
    const records = ref([])
    const selectedZone = ref('')
    const loading = ref(false)
    const totalRecords = ref(0)
    const showTutorial = ref(false)
    const showForm = ref(false)
    const editingRecord = ref(null)
    const saving = ref(false)
    const form = ref({ type: 'CNAME', name: '', content: '', proxied: false })
    const showDeleteConfirm = ref(false)
    const deletingRecord = ref(null)

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

    const openCreateModal = () => {
      editingRecord.value = null
      form.value = { type: 'CNAME', name: '', content: '', proxied: false }
      showForm.value = true
    }

    const openEditModal = (record) => {
      editingRecord.value = record
      form.value = {
        type: record.type,
        name: record.name,
        content: record.content,
        proxied: record.proxied === true
      }
      showForm.value = true
    }

    const closeForm = () => {
      showForm.value = false
      editingRecord.value = null
    }

    const saveRecord = async () => {
      if (!selectedZone.value) return
      saving.value = true
      try {
        if (editingRecord.value) {
          await api.updateDNSRecord(selectedZone.value, editingRecord.value.id, {
            type: form.value.type,
            name: form.value.name,
            content: form.value.content,
            proxied: form.value.type !== 'TXT' ? form.value.proxied : undefined
          })
          showToast('DNS record updated')
        } else {
          await api.createDNSRecord({
            zone_id: selectedZone.value,
            type: form.value.type,
            name: form.value.name,
            content: form.value.content,
            proxied: form.value.type !== 'TXT' ? form.value.proxied : false
          })
          showToast('DNS record created')
        }
        closeForm()
        loadRecords()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
      saving.value = false
    }

    const deleteRecord = (zoneId, recordId, name) => {
      deletingRecord.value = { zone_id: zoneId, id: recordId, name }
      showDeleteConfirm.value = true
    }

    const confirmDelete = async () => {
      if (!deletingRecord.value) return
      saving.value = true
      try {
        await api.deleteDNSRecord(deletingRecord.value.zone_id, deletingRecord.value.id)
        showToast('DNS record deleted')
        showDeleteConfirm.value = false
        deletingRecord.value = null
        loadRecords()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
      saving.value = false
    }

    const refresh = () => {
      loadDomains()
      if (selectedZone.value) loadRecords()
    }

    const goToTunnel = (tunnelName) => {
      router.push({ path: '/tunnels', query: { highlight: tunnelName } })
    }

    onMounted(loadDomains)

    return {
      domains, records, selectedZone, loading, totalRecords, showTutorial,
      showForm, editingRecord, saving, form,
      showDeleteConfirm, deletingRecord,
      loadRecords, openCreateModal, openEditModal, closeForm, saveRecord,
      deleteRecord, confirmDelete, refresh, goToTunnel
    }
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
    grid-template-columns: 70px 2fr 2.5fr 90px 80px 1.5fr 80px;
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

.badge.proxied {
  font-size: 0.7rem;
  padding: 0.15rem 0.5rem;
  background: rgba(243, 128, 32, 0.12);
  color: var(--accent);
  border-radius: 4px;
  font-weight: 600;
}

.badge.unproxied {
  font-size: 0.7rem;
  padding: 0.15rem 0.5rem;
  background: var(--bg-tertiary);
  color: var(--text-muted);
  border-radius: 4px;
  font-weight: 600;
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

.toggle-label {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  cursor: pointer;
  user-select: none;
}

.toggle-track {
  position: relative;
  width: 40px;
  height: 22px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 999px;
  transition: background 0.2s, border-color 0.2s;
  flex-shrink: 0;
}

.toggle-track input {
  position: absolute;
  opacity: 0;
  width: 100%;
  height: 100%;
  margin: 0;
  cursor: pointer;
  z-index: 1;
}

.toggle-thumb {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  background: var(--text-muted);
  border-radius: 50%;
  transition: transform 0.2s, background 0.2s;
  pointer-events: none;
}

.toggle-track:has(input:checked) {
  background: var(--accent);
  border-color: var(--accent);
}

.toggle-track:has(input:checked) .toggle-thumb {
  transform: translateX(18px);
  background: white;
}

.toggle-text {
  font-size: 0.875rem;
  color: var(--text-secondary);
  font-weight: 500;
}

.btn-icon {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.15s;
}

.btn-icon:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border-color: var(--border-light);
}

.btn-icon.danger:hover {
  color: var(--error);
  background: var(--error-subtle);
  border-color: var(--error);
}

.col-actions {
  display: flex;
  gap: 0.25rem;
  align-items: center;
}

@media (max-width: 768px) {
  .table-header, .table-row {
    grid-template-columns: 60px 1.5fr 2fr 60px;
  }
  .col-ttl, .col-tunnel { display: none; }
}
</style>
