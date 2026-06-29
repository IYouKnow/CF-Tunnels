<template>
  <div class="tunnels-page">
    <div class="page-header">
      <div class="header-left">
        <h2>Tunnels</h2>
        <p class="subtitle">Manage your Cloudflare Tunnels</p>
      </div>
      <div class="header-actions">
        <input v-model="searchQuery" type="text" placeholder="Search tunnels..." class="search-input" />
        <button class="btn-secondary" @click="syncTunnels" :disabled="syncing">{{ syncing ? 'Syncing...' : 'Sync' }}</button>
        <button class="btn-primary" @click="showCreateModal = true">+ New Tunnel</button>
      </div>
    </div>

    <div class="card">
      <div class="table-header">
        <div class="col-name">Name</div>
        <div class="col-domain">Domain</div>
        <div class="col-address">Address</div>
        <div class="col-uuid">UUID</div>
        <div class="col-status">Status</div>
        <div class="col-actions">Actions</div>
      </div>
      <div v-for="tunnel in filteredTunnels" :key="tunnel.id" :id="'tunnel-' + tunnel.name" :class="['table-row', { highlighted: highlightedTunnel === tunnel.name }]">
        <div class="col-name">
          <div class="tunnel-name">{{ tunnel.name }}</div>
        </div>
        <div class="col-domain">
          <span v-if="getDomainName(tunnel.zone_id)" class="domain-badge">
            {{ tunnel.subdomain }}.{{ getDomainName(tunnel.zone_id) }}
          </span>
          <span v-else class="no-domain">-</span>
        </div>
        <div class="col-address">
          <code v-if="tunnel.address">{{ tunnel.address }}</code>
          <span v-else class="no-domain">-</span>
        </div>
        <div class="col-uuid">
          <code>{{ tunnel.uuid || '-' }}</code>
        </div>
        <div class="col-status">
          <span :class="['badge', tunnel.status]">{{ tunnel.status }}</span>
        </div>
        <div class="col-actions">
          <div class="dropdown" @click.stop>
            <button class="dropdown-trigger" @click="toggleDropdown(tunnel.id)">⋮</button>
            <div v-if="openDropdown === tunnel.id" class="dropdown-menu" @click="openDropdown = null">
              <button class="dropdown-item" @click="openEditModal(tunnel)">Edit</button>
              <button v-if="tunnel.status === 'stopped'" class="dropdown-item" @click="startTunnel(tunnel.id)">Start</button>
              <button v-else class="dropdown-item" @click="stopTunnel(tunnel.id)">Stop</button>
              <button class="dropdown-item danger" @click="deleteTunnel(tunnel)">Delete</button>
            </div>
          </div>
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
          <div class="form-group">
            <label>Destination Address (optional)</label>
            <input v-model="newTunnel.address" type="text" placeholder="http://localhost:38427" />
            <small>The target service to tunnel to (e.g., http://localhost:38427, tcp://localhost:22)</small>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="showCreateModal = false">Cancel</button>
            <button type="submit" class="btn-primary">Create Tunnel</button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="showEditModal" class="modal-overlay" @click.self="showEditModal = false">
      <div class="modal">
        <h2>Edit Tunnel</h2>
        <form @submit.prevent="saveRename">
          <div class="form-group">
            <label>Tunnel Name</label>
            <input v-model="editName" type="text" placeholder="my-tunnel" required />
          </div>
          <div class="modal-actions">
            <button type="button" class="btn-secondary" @click="showEditModal = false">Cancel</button>
            <button type="submit" class="btn-primary">Save</button>
          </div>
        </form>
      </div>
    </div>

    <ConfirmModal
      :show="showDeleteConfirm"
      title="Delete Tunnel"
      :message="`Are you sure you want to delete tunnel &quot;${deletingTunnelName}&quot;?`"
      confirm-text="Delete"
      danger
      @confirm="doDeleteTunnel"
      @cancel="showDeleteConfirm = false"
    />
  </div>
</template>

<script>
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'
import { showToast } from '../toast'
import ConfirmModal from '../components/ConfirmModal.vue'

let syncedOnce = false

export default {
  name: 'Tunnels',
  components: { ConfirmModal },
  setup() {
    const tunnels = ref([])
    const domains = ref([])
    const showCreateModal = ref(false)
    const showEditModal = ref(false)
    const editTarget = ref(null)
    const editName = ref('')
    const newTunnel = ref({ name: '', account_id: '', zone_id: '', subdomain: '', address: '' })
    const showDeleteConfirm = ref(false)
    const deletingTunnelId = ref(null)
    const deletingTunnelName = ref('')
    const currentPage = ref(1)
    const perPage = ref(20)
    const totalTunnels = ref(0)
    const syncing = ref(false)
    const openDropdown = ref(null)
    const searchQuery = ref('')
    const highlightedTunnel = ref(null)
    const route = useRoute()

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

    const syncTunnels = async () => {
      syncing.value = true
      try {
        const result = await api.syncTunnels()
        const parts = []
        if (result.imported > 0) parts.push(`imported ${result.imported}`)
        if (result.updated > 0) parts.push(`updated ${result.updated}`)
        if (parts.length) {
          showToast(`Sync complete: ${parts.join(', ')} tunnel(s)`)
        } else {
          showToast('All tunnels are up to date', 'info')
        }
        loadTunnels()
      } catch (e) {
        showToast('Sync failed: ' + (e.response?.data?.error || e.message), 'error')
      } finally {
        syncing.value = false
      }
    }

    const toggleDropdown = (id) => {
      openDropdown.value = openDropdown.value === id ? null : id
    }

    const openEditModal = (tunnel) => {
      editTarget.value = tunnel
      editName.value = tunnel.name
      showEditModal.value = true
    }

    const saveRename = async () => {
      const name = editName.value.trim()
      if (!name || !editTarget.value) return
      const id = editTarget.value.id
      showEditModal.value = false
      editTarget.value = null
      try {
        await api.updateTunnel(id, { name })
        loadTunnels()
        showToast('Tunnel renamed')
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
        loadTunnels()
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
      try {
        await api.createTunnel({
          name: newTunnel.value.name,
          account_id: newTunnel.value.account_id,
          zone_id: newTunnel.value.zone_id,
          domain: selectedDomainName.value,
          subdomain: newTunnel.value.subdomain,
          address: newTunnel.value.address
        })
        showCreateModal.value = false
        newTunnel.value = { name: '', account_id: '', zone_id: '', subdomain: '', address: '' }
        loadTunnels()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
    }

    const startTunnel = async (id) => {
      try {
        await api.startTunnel(id)
        loadTunnels()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
    }

    const stopTunnel = async (id) => {
      try {
        await api.stopTunnel(id)
        loadTunnels()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
    }

    const deleteTunnel = (tunnel) => {
      deletingTunnelId.value = tunnel.id
      deletingTunnelName.value = tunnel.name
      showDeleteConfirm.value = true
    }

    const doDeleteTunnel = async () => {
      if (deletingTunnelId.value == null) return
      try {
        await api.deleteTunnel(deletingTunnelId.value)
        showDeleteConfirm.value = false
        deletingTunnelId.value = null
        deletingTunnelName.value = ''
        loadTunnels()
      } catch (e) {
        showToast(e.response?.data?.error || e.message, 'error')
      }
    }

    let pollTimer

    onMounted(async () => {
      loadDomains()
      if (!syncedOnce) {
        syncedOnce = true
        await syncTunnels()
      }
      await loadTunnels()
      pollTimer = setInterval(loadTunnels, 60000)
      if (route.query.highlight) {
        highlightedTunnel.value = route.query.highlight
        await nextTick()
        const el = document.getElementById('tunnel-' + highlightedTunnel.value)
        if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }
    })

    onUnmounted(() => {
      if (pollTimer) clearInterval(pollTimer)
      if (typeof window !== 'undefined') {
        window.removeEventListener('click', closeDropdown)
      }
    })

    const closeDropdown = () => { openDropdown.value = null }

    if (typeof window !== 'undefined') {
      window.addEventListener('click', closeDropdown)
    }

    return { 
      tunnels, 
      filteredTunnels,
      domains, 
      showCreateModal,
      showEditModal,
      editName,
      newTunnel, 
      createTunnel, 
      startTunnel, 
      stopTunnel, 
      deleteTunnel,
      doDeleteTunnel,
      showDeleteConfirm,
      deletingTunnelId,
      deletingTunnelName,
      selectedDomainName,
      getDomainName,
      currentPage,
      totalPages,
      nextPage,
      prevPage,
      searchQuery,
      highlightedTunnel,
      syncing,
      syncTunnels,
      openDropdown,
      toggleDropdown,
      openEditModal,
      saveRename
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
  grid-template-columns: 1.5fr 1.5fr 1.5fr 2fr 100px 60px;
  gap: 1rem;
  padding: 1rem 1.25rem;
  background: var(--bg-tertiary);
  font-weight: 500;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.table-row {
  display: grid;
  grid-template-columns: 1.5fr 1.5fr 1.5fr 2fr 100px 60px;
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

.table-row.highlighted {
  background: var(--accent-subtle);
  border-left: 3px solid var(--accent);
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

.col-address code {
  font-size: 0.75rem;
  color: var(--text-secondary);
  background: var(--bg-tertiary);
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.col-actions {
  position: relative;
}

.dropdown-trigger {
  background: none;
  border: 1px solid transparent;
  color: var(--text-secondary);
  font-size: 1.25rem;
  line-height: 1;
  padding: 0.25rem 0.5rem;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
  letter-spacing: 2px;
}

.dropdown-trigger:hover {
  color: var(--text-primary);
  background: var(--bg-tertiary);
  border-color: var(--border);
}

.dropdown-menu {
  position: absolute;
  right: 0;
  top: 100%;
  min-width: 120px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0,0,0,0.3);
  z-index: 20;
  overflow: hidden;
}

.dropdown-item {
  display: block;
  width: 100%;
  padding: 0.5rem 1rem;
  background: none;
  border: none;
  color: var(--text-primary);
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
  transition: background 0.1s;
}

.dropdown-item:hover {
  background: var(--bg-tertiary);
}

.dropdown-item.danger {
  color: var(--error);
}

.dropdown-item.danger:hover {
  background: rgba(239, 68, 68, 0.1);
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
  .col-uuid,
  .col-address {
    display: none;
  }
}
</style>
