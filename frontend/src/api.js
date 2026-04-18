import axios from 'axios'

const inferredDevBase = window.location.port === '8080'
  ? `${window.location.protocol}//${window.location.hostname}:3000`
  : ''

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || inferredDevBase,
  timeout: 10000,
  withCredentials: true
})

function expectLogArray (data, label) {
  if (Array.isArray(data)) return data
  if (typeof data === 'string' && data.trimStart().startsWith('<')) {
    throw new Error(`${label}: server returned a web page instead of JSON — use the same origin as the API (e.g. Vite dev server with /api proxy, or the Go server port) and hard-refresh`)
  }
  throw new Error(`${label}: expected a JSON array from the server`)
}

client.interceptors.response.use(
  response => response,
  error => {
    console.error('API Error:', error.response?.status, error.response?.data)
    if (error.response?.status === 401 && !error.config?.skipAuthRedirect) {
      import('./router').then(({ default: router }) => {
        if (router.currentRoute.value.path !== '/login') {
          router.replace({ path: '/login', query: { redirect: router.currentRoute.value.fullPath } })
        }
      })
    }
    return Promise.reject(error)
  }
)

export default {
  login: (username, password) =>
    client.post('/api/login', { username, password }, { skipAuthRedirect: true }).then(r => r.data),
  logout: () => client.post('/api/logout').then(r => r.data),
  getMe: () => client.get('/api/auth/me', { skipAuthRedirect: true }).then(r => r.data),

  getStatus: () => client.get('/api/status').then(r => r.data),
  getTunnels: () => client.get('/api/tunnels').then(r => r.data),
  getTunnel: (id) => client.get(`/api/tunnels/${id}`).then(r => r.data),
  createTunnel: (data) => client.post('/api/tunnels', data).then(r => r.data),
  deleteTunnel: (id) => client.delete(`/api/tunnels/${id}`).then(r => r.data),
  startTunnel: (id) => client.post(`/api/tunnels/${id}/start`).then(r => r.data),
  stopTunnel: (id) => client.post(`/api/tunnels/${id}/stop`).then(r => r.data),
  getTunnelLogs: (id, limit = 100) =>
    client.get(`/api/tunnels/${id}/logs?limit=${limit}`).then(r => expectLogArray(r.data, 'Tunnel logs')),
  getAllLogs: (limit = 500) =>
    client.get(`/api/logs?limit=${limit}`).then(r => expectLogArray(r.data, 'All logs')),
  getIngressRules: (tunnelId) => client.get(`/api/ingress?tunnel_id=${tunnelId}`).then(r => r.data),
  createIngressRule: (data) => client.post('/api/ingress', data).then(r => r.data),
  updateIngressRule: (id, data) => client.put(`/api/ingress/${id}`, data).then(r => r.data),
  deleteIngressRule: (id) => client.delete(`/api/ingress/${id}`).then(r => r.data),
  getDomains: (page = 1, perPage = 50) => client.get(`/api/domains?page=${page}&per_page=${perPage}`).then(r => r.data),
  createDNSRecord: (data) => client.post('/api/dns', data).then(r => r.data),
  deleteDNSRecord: (zoneId, recordId) => client.delete(`/api/dns/${zoneId}/${recordId}`).then(r => r.data)
}
