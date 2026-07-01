<template>
  <div v-if="show" class="modal-overlay" @mousedown.self="$emit('cancel')">
    <div class="modal confirm-modal">
      <div class="modal-header">
        <h2>{{ title }}</h2>
        <button class="close-btn" @click="$emit('cancel')">&times;</button>
      </div>
      <p class="confirm-text">{{ message }}</p>
      <div class="modal-actions">
        <button class="btn-secondary" @click="$emit('cancel')">Cancel</button>
        <button :class="danger ? 'btn-danger' : 'btn-primary'" @click="$emit('confirm')" :disabled="disabled">
          {{ loading ? loadingText || confirmText : confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<script>
import { onMounted, onUnmounted } from 'vue'

export default {
  name: 'ConfirmModal',
  props: {
    show: Boolean,
    title: { type: String, default: 'Confirm' },
    message: { type: String, default: 'Are you sure?' },
    confirmText: { type: String, default: 'Confirm' },
    loadingText: { type: String, default: '' },
    danger: Boolean,
    disabled: Boolean,
    loading: Boolean
  },
  emits: ['confirm', 'cancel'],
  setup (props, { emit }) {
    function onKeyDown(e) {
      if (e.key === 'Enter' && props.show && !props.disabled) {
        emit('confirm')
      }
    }

    onMounted(() => window.addEventListener('keydown', onKeyDown))
    onUnmounted(() => window.removeEventListener('keydown', onKeyDown))
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
  max-width: 480px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: var(--shadow-lg);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.modal-header h2 {
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0;
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

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 1.5rem;
}

.confirm-text {
  margin: 1rem 0 1.5rem;
  font-size: 0.9rem;
  color: var(--text-secondary);
  line-height: 1.5;
}

.btn-secondary {
  background: transparent;
  border: 1px solid var(--border);
  padding: 0.5rem 1rem;
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-weight: 500;
  font-size: 0.875rem;
  cursor: pointer;
  transition: all 0.15s;
  line-height: 1.4;
}

.btn-secondary:hover {
  background: var(--bg-tertiary);
}

.btn-primary {
  background: var(--accent);
  color: white;
  border: none;
  padding: 0.5rem 1rem;
  border-radius: var(--radius-md);
  font-weight: 500;
  font-size: 0.875rem;
  cursor: pointer;
  transition: background 0.15s;
  line-height: 1.4;
}

.btn-primary:hover {
  background: var(--accent-hover);
}

.btn-danger {
  background: var(--error);
  color: white;
  border: none;
  padding: 0.5rem 1rem;
  border-radius: var(--radius-md);
  font-weight: 500;
  font-size: 0.875rem;
  cursor: pointer;
  transition: background 0.15s;
  line-height: 1.4;
}

.btn-danger:hover {
  background: #dc2626;
}

.btn-danger:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}
</style>
