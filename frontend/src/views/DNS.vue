<template>
  <div class="dns-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Domains & DNS Records</h2>
        <p class="subtitle">Manage your Cloudflare zones and DNS records</p>
      </div>
      <div class="header-actions">
        <button class="btn-secondary" @click="showTutorial = true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18">
            <circle cx="12" cy="12" r="10"/>
            <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/>
            <line x1="12" y1="17" x2="12.01" y2="17"/>
          </svg>
          API Token Help
        </button>
        <button class="btn-primary" @click="loadDomains">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="18" height="18">
            <polyline points="23 4 23 10 17 10"/>
            <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/>
          </svg>
          Refresh
        </button>
      </div>
    </div>

    <div v-if="showTutorial" class="modal-overlay" @click.self="showTutorial = false">
      <div class="modal tutorial-modal">
        <div class="modal-header">
          <h2>Creating a Cloudflare API Token</h2>
          <button class="close-btn" @click="showTutorial = false">&times;</button>
        </div>
        <div class="tutorial-content">
          <p class="intro">To manage domains and DNS records, you need a Cloudflare API token with the following permissions:</p>
          
<div class="permissions-list">
            <h3>Required Permissions</h3>
            <ul>
              <li><strong>Account DNS Settings - Read</strong> - View DNS settings</li>
              <li><strong>Account DNS Settings - Edit</strong> - Modify DNS settings</li>
              <li><strong>DNS View - Read</strong> - View DNS records</li>
              <li><strong>DNS Firewall - Read</strong> - View DNS firewall settings</li>
              <li><strong>Registrar Domains - Read</strong> - View domain registration info</li>
            </ul>
          </div>

<div class="steps">
            <h3>How to create the token:</h3>
            <ol>
              <li>Go to <strong>Cloudflare Dashboard</strong></li>
              <li>Click on your profile icon (top right) → <strong>Profile</strong></li>
              <li>Scroll down to <strong>API Tokens</strong></li>
              <li>Click <strong>Create Custom Token</strong></li>
              <li>Give it a name (e.g., "CF Tunnel Dashboard")</li>
              <li>Under <strong>Account Resources</strong>, select <strong>Include</strong> → <strong>Entire account</strong> (or specific account)</li>
              <li>Under <strong>Permissions</strong>, click <strong>Add</strong> → Select <strong>DNS & Zones</strong></li>
              <li>In the dropdown, select all of these:
                <ul class="sub-steps">
                  <li>Account DNS Settings - Read</li>
                  <li>Account DNS Settings - Edit</li>
                  <li>DNS View - Read</li>
                  <li>DNS Firewall - Read</li>
                  <li>Registrar Domains - Read</li>
                </ul>
              </li>
              <li>Under <strong>Token expiration</strong>, choose an expiration period (e.g., "No expiration" or "30 days")</li>
              <li>Leave <strong>Client IP Address Filtering</strong> as default (optional)</li>
              <li>Click <strong>Continue to summary</strong></li>
              <li>Click <strong>Create Token</strong></li>
              <li><strong>Copy the token</strong> and add it to your <code>.env</code> file as <code>CF_API_TOKEN</code></li>
            </ol>
          </div>

          <div class="env-example">
            <h3>Your .env file should look like:</h3>
            <pre>CF_API_TOKEN=your_new_token_here
CF_ACCOUNT_ID=your_account_id</pre>
          </div>
        </div>
      </div>
    </div>

    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ totalDomains }}</div>
        <div class="stat-label">Total Domains</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ currentPage }} / {{ totalPages }}</div>
        <div class="stat-label">Page</div>
      </div>
    </div>

    <div class="card">
      <div class="card-header">
        <span>Your Domains</span>
        <div class="pagination-controls">
          <button class="btn-small" @click="prevPage" :disabled="currentPage <= 1">Prev</button>
          <span class="page-info">{{ currentPage }} - {{ Math.min(currentPage * perPage, totalDomains) }}</span>
          <button class="btn-small" @click="nextPage" :disabled="currentPage >= totalPages">Next</button>
        </div>
      </div>
      <div class="domain-list" v-if="domains.length > 0">
        <div v-for="domain in domains" :key="domain.id" class="domain-item">
          <div class="domain-info">
            <div class="domain-name">{{ domain.name }}</div>
            <div class="domain-id">ID: {{ domain.id }}</div>
          </div>
          <div class="domain-status">
            <span class="badge running">Active</span>
          </div>
        </div>
      </div>
      <div v-else class="empty">
        <div v-if="loading">Loading domains...</div>
        <div v-else>No domains found. Add a domain to Cloudflare to get started.</div>
      </div>
    </div>

    <div class="card" style="margin-top: 1.5rem;">
      <div class="card-header">Quick DNS Record</div>
      <div class="dns-form">
        <div class="form-row">
          <div class="form-group">
            <label>Zone</label>
            <select v-model="newRecord.zone_id">
              <option value="">Select domain</option>
              <option v-for="d in domains" :key="d.id" :value="d.id">{{ d.name }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>Type</label>
            <select v-model="newRecord.type">
              <option value="CNAME">CNAME</option>
              <option value="A">A</option>
              <option value="AAAA">AAAA</option>
              <option value="TXT">TXT</option>
            </select>
          </div>
          <div class="form-group">
            <label>Name</label>
            <input v-model="newRecord.name" type="text" placeholder="subdomain" />
          </div>
          <div class="form-group">
            <label>Content</label>
            <input v-model="newRecord.content" type="text" placeholder="target.domain.com" />
          </div>
          <button class="btn-primary" @click="createRecord" :disabled="!newRecord.zone_id || !newRecord.name">
            Add Record
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, computed, onMounted } from 'vue'
import api from '../api'

export default {
  name: 'DNS',
  setup() {
    const domains = ref([])
    const loading = ref(false)
    const totalDomains = ref(0)
    const currentPage = ref(1)
    const perPage = ref(50)
    const showTutorial = ref(false)
    
    const newRecord = ref({
      zone_id: '',
      type: 'CNAME',
      name: '',
      content: ''
    })

    const totalPages = computed(() => Math.ceil(totalDomains.value / perPage.value))

    const loadDomains = async () => {
      loading.value = true
      try {
        const result = await api.getDomains(currentPage.value, perPage.value)
        domains.value = result.domains || []
        totalDomains.value = result.total || 0
      } catch (e) {
        console.error('Failed to load domains:', e)
      }
      loading.value = false
    }

    const nextPage = () => {
      if (currentPage.value < totalPages.value) {
        currentPage.value++
        loadDomains()
      }
    }

    const prevPage = () => {
      if (currentPage.value > 1) {
        currentPage.value--
        loadDomains()
      }
    }

    const createRecord = async () => {
      if (!newRecord.value.zone_id || !newRecord.value.name) return
      try {
        await api.createDNSRecord({
          zone_id: newRecord.value.zone_id,
          type: newRecord.value.type,
          name: newRecord.value.name,
          content: newRecord.value.content
        })
        newRecord.value = { zone_id: '', type: 'CNAME', name: '', content: '' }
        alert('DNS record created!')
      } catch (e) {
        alert('Failed to create DNS record: ' + e.message)
      }
    }

    onMounted(loadDomains)
    return { 
      domains, 
      loading, 
      totalDomains, 
      currentPage, 
      perPage, 
      totalPages,
      newRecord, 
      showTutorial,
      loadDomains, 
      nextPage, 
      prevPage, 
      createRecord 
    }
  }
}
</script>

<style scoped>
.dns-page {
  max-width: 1200px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
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

.btn-primary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.pagination-controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.btn-small {
  padding: 0.375rem 0.75rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 6px;
  color: var(--text-primary);
  font-size: 0.8rem;
  cursor: pointer;
}

.btn-small:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-small:hover:not(:disabled) {
  background: var(--border);
}

.page-info {
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.domain-list {
  display: flex;
  flex-direction: column;
}

.domain-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--border);
}

.domain-item:last-child {
  border-bottom: none;
}

.domain-item:hover {
  background: var(--bg-tertiary);
}

.domain-name {
  font-weight: 500;
  font-size: 1rem;
}

.domain-id {
  font-size: 0.8rem;
  color: var(--text-secondary);
  font-family: monospace;
  margin-top: 0.25rem;
}

.dns-form {
  padding: 1rem 1.25rem;
}

.form-row {
  display: flex;
  gap: 1rem;
  align-items: flex-end;
  flex-wrap: wrap;
}

.form-row .form-group {
  flex: 1;
  min-width: 150px;
  margin-bottom: 0;
}

.form-row .btn-primary {
  flex: 0;
  white-space: nowrap;
}

.header-actions {
  display: flex;
  gap: 0.75rem;
}

.btn-secondary {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  padding: 0.625rem 1.25rem;
  border-radius: 8px;
  color: var(--text-primary);
  font-weight: 500;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-secondary:hover {
  background: var(--border);
}

.tutorial-modal {
  max-width: 600px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
}

.tutorial-modal .modal {
  max-height: 85vh;
  display: flex;
  flex-direction: column;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  flex-shrink: 0;
}

.modal-header h2 {
  font-size: 1.25rem;
  font-weight: 600;
}

.close-btn {
  background: none;
  border: none;
  font-size: 1.5rem;
  color: var(--text-secondary);
  cursor: pointer;
  padding: 0.25rem;
  line-height: 1;
}

.close-btn:hover {
  color: var(--text-primary);
}

.tutorial-content {
  color: var(--text-primary);
  overflow-y: auto;
  padding-right: 0.5rem;
}

.tutorial-content::-webkit-scrollbar {
  width: 6px;
}

.tutorial-content::-webkit-scrollbar-track {
  background: var(--bg-tertiary);
  border-radius: 3px;
}

.tutorial-content::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 3px;
}

.tutorial-content::-webkit-scrollbar-thumb:hover {
  background: var(--text-secondary);
}

.tutorial-content .intro {
  margin-bottom: 1.25rem;
  color: var(--text-secondary);
  line-height: 1.5;
}

.permissions-list {
  background: var(--bg-tertiary);
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 1.25rem;
  border: 1px solid var(--border);
}

.permissions-list h3 {
  font-size: 0.95rem;
  margin-bottom: 0.75rem;
  font-weight: 600;
}

.permissions-list ul {
  list-style: none;
  padding: 0;
  margin: 0;
}

.permissions-list li {
  padding: 0.375rem 0;
  font-size: 0.9rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.permissions-list li::before {
  content: "✓";
  color: var(--success);
  font-weight: bold;
}

.steps {
  margin-bottom: 1.25rem;
}

.steps h3 {
  font-size: 0.95rem;
  margin-bottom: 0.75rem;
  font-weight: 600;
}

.steps ol {
  padding-left: 1.25rem;
  margin: 0;
}

.steps li {
  margin-bottom: 0.5rem;
  font-size: 0.9rem;
  line-height: 1.5;
}

.steps .sub-steps {
  margin-top: 0.5rem;
  padding-left: 1rem;
}

.steps .sub-steps li {
  color: var(--text-secondary);
  list-style: disc;
}

.steps .sub-steps li::before {
  display: none;
}

.env-example {
  background: var(--bg-tertiary);
  border-radius: 8px;
  padding: 1rem;
  border: 1px solid var(--border);
}

.env-example h3 {
  font-size: 0.9rem;
  margin-bottom: 0.75rem;
  font-weight: 600;
}

.env-example pre {
  font-family: 'Monaco', 'Menlo', 'Courier New', monospace;
  font-size: 0.85rem;
  color: var(--text-secondary);
  margin: 0;
  background: var(--bg-primary);
  padding: 0.75rem;
  border-radius: 6px;
  border: 1px solid var(--border);
}

@media (max-width: 640px) {
  .form-row {
    flex-direction: column;
  }
  .form-row .form-group {
    width: 100%;
  }
}
</style>