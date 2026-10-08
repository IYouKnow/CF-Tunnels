<template>
  <div v-if="show" class="modal-overlay" @mousedown.self="close">
    <div class="modal domains-modal">
      <div class="modal-header">
        <h2>Domains <span class="tunnel-name">— {{ tunnelName }}</span></h2>
        <button class="close-btn" @click="close">&times;</button>
      </div>

      <p class="hint">
        The <strong>primary</strong> domain comes from the tunnel's subdomain + domain settings.
        Extra domains route through this tunnel's connector; DNS CNAMEs are created and removed automatically.
      </p>

      <div v-if="loading" class="loading">Loading domains…</div>

      <div v-else class="domain-list">
        <div v-if="primaryHost" class="domain-row primary">
          <div class="domain-main">
            <code>{{ primaryHost }}</code>
            <span class="tag">primary</span>
            <div class="domain-sub">{{ tunnel.address || 'no destination address' }}</div>
          </div>
        </div>

        <div v-for="rule in rules" :key="rule.id" class="domain-row">
          <template v-if="editingId === rule.id">
            <div class="edit-grid">
              <input v-model="editForm.hostname" type="text" placeholder="app.example.com" />
              <input v-model="editForm.service" type="text" :placeholder="tunnel.address || 'http://host:port'" />
              <input v-model="editForm.path" type="text" placeholder="/path (optional)" />
              <div class="edit-actions">
                <button class="btn-secondary" @click="cancelEdit">Cancel</button>
                <button class="btn-primary" :disabled="saving || !editForm.hostname.trim()" @click="saveEdit(rule)">Save</button>
              </div>
            </div>
          </template>
          <template v-else>
            <div class="domain-main">
              <code>{{ rule.hostname }}<span v-if="rule.path" class="path">{{ rule.path }}</span></code>
              <div class="domain-sub">{{ rule.service }}</div>
            </div>
            <div class="row-actions">
              <button class="icon-btn" @click="startEdit(rule)">Edit</button>
              <button class="icon-btn danger" @click="askDelete(rule)">Delete</button>
            </div>
          </template>
        </div>

        <div class="domain-row add-row">
          <div class="edit-grid">
            <input v-model="newForm.hostname" type="text" placeholder="app.example.com" @keyup.enter="addDomain" />
            <input v-model="newForm.service" type="text" :placeholder="tunnel.address || 'http://host:port'" />
            <input v-model="newForm.path" type="text" placeholder="/path (optional)" @keyup.enter="addDomain" />
            <div class="edit-actions">
              <button class="btn-primary" :disabled="saving || !newForm.hostname.trim()" @click="addDomain">
                {{ saving ? 'Saving…' : 'Add domain' }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <p v-if="error" class="error-text">{{ error }}</p>

      <ConfirmModal
        :show="showDelete"
        title="Remove Domain"
        :message="`Remove ${deletingRule ? deletingRule.hostname : ''}? Its DNS CNAME will also be deleted.`"
        confirm-text="Delete"
        danger
        :loading="saving"
        @confirm="confirmDelete"
        @cancel="showDelete = false"
      />
    </div>
  </div>
</template>

<script>
import { ref, computed, watch } from 'vue'
import api from '../api'
import { showToast } from '../toast'
import ConfirmModal from './ConfirmModal.vue'

export default {
  name: 'DomainsModal',
  components: { ConfirmModal },
  props: {
    show: Boolean,
    tunnel: { type: Object, default: null }
  },
  emits: ['close', 'updated'],
  setup (props, { emit }) {
    const rules = ref([])
    const loading = ref(false)
    const saving = ref(false)
    const error = ref('')
    const newForm = ref({ hostname: '', service: '', path: '' })
    const editingId = ref(null)
    const editForm = ref({ hostname: '', service: '', path: '' })
    const showDelete = ref(false)
    const deletingRule = ref(null)

    const tunnelName = computed(() => props.tunnel?.name || '')
    const primaryHost = computed(() => {
      if (!props.tunnel) return ''
      const sub = (props.tunnel.subdomain || '').trim()
      const dom = (props.tunnel.domain || '').trim()
      return sub && dom ? `${sub}.${dom}` : ''
    })

    const resetForms = () => {
      newForm.value = { hostname: '', service: '', path: '' }
      editingId.value = null
      editForm.value = { hostname: '', service: '', path: '' }
      error.value = ''
    }

    const loadRules = async () => {
      if (!props.tunnel) return
      loading.value = true
      error.value = ''
      try {
        rules.value = await api.getIngressRules(props.tunnel.id)
      } catch (e) {
        error.value = e.response?.data?.error || e.message
        rules.value = []
      } finally {
        loading.value = false
      }
    }

    const refresh = () => {
      loadRules()
      emit('updated')
    }

    const addDomain = async () => {
      const hostname = newForm.value.hostname.trim()
      if (!hostname || !props.tunnel) return
      saving.value = true
      error.value = ''
      try {
        const res = await api.createIngressRule({
          tunnel_id: props.tunnel.id,
          hostname,
          service: newForm.value.service.trim(),
          path: newForm.value.path.trim(),
          protocol: 'http'
        })
        if (res?.dnsWarning) showToast(res.dnsWarning, 'info')
        else showToast('Domain added')
        resetForms()
        refresh()
      } catch (e) {
        error.value = e.response?.data?.error || e.message
        showToast(error.value, 'error')
      } finally {
        saving.value = false
      }
    }

    const startEdit = (rule) => {
      editingId.value = rule.id
      editForm.value = { hostname: rule.hostname || '', service: rule.service || '', path: rule.path || '' }
    }

    const cancelEdit = () => {
      editingId.value = null
      editForm.value = { hostname: '', service: '', path: '' }
    }

    const saveEdit = async (rule) => {
      const hostname = editForm.value.hostname.trim()
      if (!hostname) return
      saving.value = true
      error.value = ''
      try {
        const res = await api.updateIngressRule(rule.id, {
          tunnel_id: rule.tunnel_id,
          hostname,
          service: editForm.value.service.trim(),
          path: editForm.value.path.trim(),
          protocol: rule.protocol || 'http'
        })
        if (res?.dnsWarning) showToast(res.dnsWarning, 'info')
        else showToast('Domain updated')
        cancelEdit()
        refresh()
      } catch (e) {
        error.value = e.response?.data?.error || e.message
        showToast(error.value, 'error')
      } finally {
        saving.value = false
      }
    }

    const askDelete = (rule) => {
      deletingRule.value = rule
      showDelete.value = true
    }

    const confirmDelete = async () => {
      if (!deletingRule.value) return
      saving.value = true
      error.value = ''
      try {
        await api.deleteIngressRule(deletingRule.value.id)
        showToast('Domain removed')
        showDelete.value = false
        deletingRule.value = null
        refresh()
      } catch (e) {
        error.value = e.response?.data?.error || e.message
        showToast(error.value, 'error')
      } finally {
        saving.value = false
      }
    }

    const close = () => {
      resetForms()
      emit('close')
    }

    watch(
      () => [props.show, props.tunnel?.id],
      ([visible]) => {
        if (visible) {
          resetForms()
          loadRules()
        }
      }
    )

    return {
      rules,
      loading,
      saving,
      error,
      newForm,
      editingId,
      editForm,
      showDelete,
      deletingRule,
      tunnelName,
      primaryHost,
      addDomain,
      startEdit,
      cancelEdit,
      saveEdit,
      askDelete,
      confirmDelete,
      close
    }
  }
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.modal {
  background: var(--bg-elevated);
  border: 1px solid var(--border-light);
  border-radius: var(--radius-lg);
  padding: 1.5rem;
  width: 100%;
  max-width: 640px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.75rem;
}

.modal-header h2 {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0;
}

.tunnel-name {
  color: var(--text-secondary);
  font-weight: 400;
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

.hint {
  font-size: 0.82rem;
  color: var(--text-secondary);
  line-height: 1.5;
  margin: 0 0 1rem;
}

.loading {
  color: var(--text-secondary);
  font-size: 0.9rem;
  padding: 1rem 0;
}

.domain-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.domain-row {
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: 0.75rem 0.9rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.domain-row.primary {
  border-style: dashed;
}

.domain-row.add-row {
  background: transparent;
  border-style: dashed;
}

.domain-main {
  min-width: 0;
}

.domain-main code {
  font-size: 0.9rem;
}

.domain-main .path {
  color: var(--text-secondary);
}

.domain-sub {
  font-size: 0.78rem;
  color: var(--text-secondary);
  margin-top: 0.2rem;
  word-break: break-all;
}

.tag {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--accent);
  border: 1px solid var(--accent);
  border-radius: 999px;
  padding: 0.05rem 0.45rem;
  margin-left: 0.5rem;
}

.row-actions {
  display: flex;
  gap: 0.5rem;
  flex-shrink: 0;
}

.icon-btn {
  background: transparent;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-size: 0.8rem;
  padding: 0.3rem 0.6rem;
  cursor: pointer;
}

.icon-btn:hover {
  background: var(--bg-elevated);
}

.icon-btn.danger {
  color: var(--error);
  border-color: var(--error);
}

.edit-grid {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 0.5rem;
  width: 100%;
  align-items: center;
}

.edit-grid input {
  padding: 0.5rem 0.65rem;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-primary);
  font-size: 0.85rem;
  min-width: 0;
}

.edit-grid input:focus {
  outline: none;
  border-color: var(--accent);
}

.edit-actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}

.btn-secondary,
.btn-primary {
  padding: 0.45rem 0.9rem;
  border-radius: var(--radius-md);
  font-weight: 500;
  font-size: 0.85rem;
  cursor: pointer;
  line-height: 1.4;
}

.btn-secondary {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--text-primary);
}

.btn-secondary:hover {
  background: var(--bg-tertiary);
}

.btn-primary {
  background: var(--accent);
  color: white;
  border: none;
}

.btn-primary:hover {
  background: var(--accent-hover);
}

.btn-primary:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.error-text {
  color: var(--error);
  font-size: 0.85rem;
  margin: 0.75rem 0 0;
}

@media (max-width: 640px) {
  .edit-grid {
    grid-template-columns: 1fr;
  }
}
</style>
