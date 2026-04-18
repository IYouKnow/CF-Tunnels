<template>
  <div class="login-page">
    <div class="login-card card">
      <div class="login-header">
        <h1>CF Tunnels</h1>
        <p class="subtitle">Sign in to manage tunnels and DNS</p>
      </div>
      <form @submit.prevent="submit">
        <div class="form-group">
          <label for="username">Username</label>
          <input
            id="username"
            v-model="username"
            type="text"
            autocomplete="username"
            required
          >
        </div>
        <div class="form-group">
          <label for="password">Password</label>
          <input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
          >
        </div>
        <p v-if="error" class="error-msg">{{ error }}</p>
        <button type="submit" class="btn-primary login-btn" :disabled="submitting">
          {{ submitting ? 'Signing in…' : 'Sign in' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script>
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { refreshAuth } from '../auth'

export default {
  name: 'Login',
  setup () {
    const route = useRoute()
    const router = useRouter()
    const username = ref('')
    const password = ref('')
    const error = ref('')
    const submitting = ref(false)

    const submit = async () => {
      error.value = ''
      submitting.value = true
      try {
        await api.login(username.value, password.value)
        await refreshAuth()
        const redir = route.query.redirect
        const path = typeof redir === 'string' && redir.startsWith('/') ? redir : '/'
        router.replace(path)
      } catch (e) {
        const msg = e.response?.data?.error
        error.value = typeof msg === 'string' ? msg : 'Sign in failed'
      } finally {
        submitting.value = false
      }
    }

    return { username, password, error, submitting, submit }
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  background: var(--bg-primary);
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 2rem;
}

.login-header {
  margin-bottom: 1.75rem;
  text-align: center;
}

.login-header h1 {
  font-size: 1.5rem;
  font-weight: 700;
  margin-bottom: 0.5rem;
}

.subtitle {
  font-size: 0.9rem;
  color: var(--text-secondary);
}

.error-msg {
  color: var(--error);
  font-size: 0.875rem;
  margin-bottom: 1rem;
}

.login-btn {
  width: 100%;
  margin-top: 0.25rem;
}

.login-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}
</style>
