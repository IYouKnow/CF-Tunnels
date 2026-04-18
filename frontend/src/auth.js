import { ref } from 'vue'
import api from './api'

export const currentUser = ref(null)

export async function refreshAuth () {
  try {
    const data = await api.getMe()
    currentUser.value = data.username
    return true
  } catch {
    currentUser.value = null
    return false
  }
}
