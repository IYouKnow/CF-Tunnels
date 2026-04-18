<template>
  <div class="tunnels-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Tunnels</h2>
        <p class="subtitle">Manage your Cloudflare Tunnels</p>
      </div>
      <div class="header-actions">
        <input v-model="searchQuery" type="text" placeholder="Search tunnels..." class="search-input" />
        <button class="btn-primary" @click="showCreateModal = true">+ New Tunnel</button>
      </div>
    </div>

    <div class="card">
      <div class="table-header">
        <div class="col-name">Name</div>
        <div class="col-domain">Domain</div>
        <div class="col-uuid">UUID</div>
        <div class="col-status">Status</div>
        <div class="col-actions">Actions</div>
      </div>
      <div v-for="tunnel in filteredTunnels" :key="tunnel.id" class="table-row">
        <div class="col-name">
          <div class="tunnel-name">{{ tunnel.name }}</div>
        </div>
        <div class="col-domain">
          <span v-if="getDomainName(tunnel.zone_id)" class="domain-badge">
            {{ tunnel.subdomain }}.{{ getDomainName(tunnel.zone_id) }}
          </span>
          <span v-else class="no-domain">-</span>
        </div>
        <div class="col-uuid">
          <code>{{ tunnel.uuid || '-' }}</code>
        </div>
        <div class="col-status">
          <span :class="['badge', tunnel.status]">{{ tunnel.status }}</span>
        </div>
        <div class="col-actions">
          <button v-if="tunnel.status === 'stopped'" class="btn-action success" @click="startTunnel(tunnel.id)">Start</button>
          <button v-else class="btn-action" @click="stopTunnel(tunnel.id)">Stop</button>
          <button class="btn-action danger" @click="deleteTunnel(tunnel.id)">Delete</button>
        </div>
      </div>
      <div v-if="tunnels.length === 0" class="empty">No tunnels. Create one to get started.</div>
      <div v-else class="table-footer">
        <div class="pagination">
          <button class="btn-small" @click="prevPage" :disabled="currentPage <= 1">Prev</button>
          <span class="page-info">Page {{ currentPage }} of {{ totalPages }}</span>
          <button class="btn-small" @click="nextPage" :disabled="currentPage >= totalPages">Next</button>
        </div>
      </div>
    </div>

    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal">
        <h2>Create Tunnel</h2>
        <form @submit.prevent="createTunnel">
          <div class="form-group">
            <label>Tunnel Name</label>
            <input v-model="newTunnel.name" type="text" placeholder="my-tunnel" required />
          </div>
          <div class="form-group">
            <label>Domain</label>
            <select v-model="newTunnel.zone_id" required>
              <option value="">Select a domain</option>
              <option v-for="domain in domains" :key="domain.id" :value="domain.id">{{ domain.name }}</option>
            </select>
          </div>
          <div class="form-group">
            <label>Subdomain (optional)</label>
            <input v-model="newTunnel.subdomain" type="text" placeholder="myapp" />
            <small v-if="newTunnel.zone_id">Will create: {{ newTunnel.subdomain || 'subdomain' }}.{{ selectedDomainName }}</small>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="showCreateModal = false">Cancel</button>
            <button type="submit" class="btn-primary">Create Tunnel</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script>
import { ref, onMounted, computed } from 'vue'
import api from '../api'

export default {
  name: 'Tunnels',
  setup() {
    const tunnels = ref([])
    const domains = ref([])
    const showCreateModal = ref(false)
    const newTunnel = ref({ name: '', account_id: '', zone_id: '', subdomain: '' })
    const currentPage = ref(1)
    const perPage = ref(20)
    const totalTunnels = ref(0)
    const searchQuery = ref('')

    const totalPages = computed(() => Math.ceil(totalTunnels.value / perPage.value))

    const filteredTunnels = computed(() => {
      if (!searchQuery.value) return tunnels.value
      const query = searchQuery.value.toLowerCase()
      return tunnels.value.filter(t => 
        t.name.toLowerCase().includes(query) ||
        (t.subdomain && t.subdomain.toLowerCase().includes(query)) ||
        (t.uuid && t.uuid.toLowerCase().includes(query))
      )
    })

    const selectedDomainName = computed(() => {
      const domain = domains.value.find(d => d.id === newTunnel.value.zone_id)
      return domain ? domain.name : ''
    })

    const getDomainName = (zoneId) => {
      const domain = domains.value.find(d => d.id === zoneId)
      return domain ? domain.name : ''
    }

    const loadTunnels = async () => {
      const allTunnels = await api.getTunnels()
      totalTunnels.value = (allTunnels || []).length
      const start = (currentPage.value - 1) * perPage.value
      tunnels.value = (allTunnels || []).slice(start, start + perPage.value)
    }

    const loadDomains = async () => {
      try {
        const result = await api.getDomains(1, 100)
        domains.value = result.domains || []
      } catch (e) {
        console.error('Failed to load domains:', e)
      }
    }

    const nextPage = () => {
      if (currentPage.value < totalPages.value) {
        currentPage.value++
        loadTunnels()
      }
    }

    const prevPage = () => {
      if (currentPage.value > 1) {
        currentPage.value--
        loadTunnels()
      }
    }

    const createTunnel = async () => {
      await api.createTunnel({
        name: newTunnel.value.name,
        account_id: newTunnel.value.account_id,
        zone_id: newTunnel.value.zone_id,
        subdomain: newTunnel.value.subdomain
      })
      showCreateModal.value = false
      newTunnel.value = { name: '', account_id: '', zone_id: '', subdomain: '' }
      loadTunnels()
    }

    const startTunnel = async (id) => {
      await api.startTunnel(id)
      loadTunnels()
    }

    const stopTunnel = async (id) => {
      await api.stopTunnel(id)
      loadTunnels()
    }

    const deleteTunnel = async (id) => {
      if (confirm('Are you sure you want to delete this tunnel?')) {
        await api.deleteTunnel(id)
        loadTunnels()
      }
    }

    onMounted(() => {
      loadTunnels()
      loadDomains()
    })
    return { 
      tunnels, 
      filteredTunnels,
      domains, 
      showCreateModal, 
      newTunnel, 
      createTunnel, 
      startTunnel, 
      stopTunnel, 
      deleteTunnel, 
      selectedDomainName,
      getDomainName,
      currentPage,
      totalPages,
      nextPage,
      prevPage,
      searchQuery
    }
  }
}
</script>

<style scoped>
.tunnels-page {
  max-width: 1200px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.header-actions {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}

.search-input {
  padding: 0.5rem 1rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-primary);
  font-size: 0.9rem;
  width: 250px;
}

.search-input:focus {
  outline: none;
  border-color: var(--accent);
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

.table-header {
  display: grid;
  grid-template-columns: 1.5fr 2fr 2fr 100px 140px;
  gap: 1rem;
  padding: 1rem 1.25rem;
  background: var(--bg-tertiary);
  font-weight: 500;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.table-row {
  display: grid;
  grid-template-columns: 1.5fr 2fr 2fr 100px 140px;
  gap: 1rem;
  padding: 1rem 1.25rem;
  align-items: center;
  border-bottom: 1px solid var(--border);
  transition: background 0.2s;
}

.table-row:last-child {
  border-bottom: none;
}

.table-row:hover {
  background: var(--bg-tertiary);
}

.col-name {
  font-weight: 500;
}

.tunnel-name {
  font-size: 0.95rem;
}

.col-uuid code {
  font-size: 0.75rem;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.domain-badge {
  font-size: 0.85rem;
  color: var(--accent);
  background: rgba(243, 128, 32, 0.1);
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.no-domain {
  color: var(--text-secondary);
}

.col-actions {
  display: flex;
  gap: 0.5rem;
}

.col-actions .btn-action {
  padding: 0.375rem 0.75rem;
  font-size: 0.8rem;
}

.table-footer {
  padding: 1rem 1.25rem;
  border-top: 1px solid var(--border);
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
}

.page-info {
  font-size: 0.85rem;
  color: var(--text-secondary);
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

@media (max-width: 900px) {
  .table-header,
  .table-row {
    grid-template-columns: 1fr 1fr 80px 120px;
  }
  .col-uuid {
    display: none;
  }
}
</style>