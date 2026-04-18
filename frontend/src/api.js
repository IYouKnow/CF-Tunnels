import axios from 'axios'

const client = axios.create({
  baseURL: '',
  timeout: 10000,
  auth: {
    username: localStorage.getItem('username') || 'admin',
    password: localStorage.getItem('password') || 'admin'
  }
})

client.interceptors.response.use(
  response => response,
  error => {
    console.error('API Error:', error.response?.status, error.response?.data)
    if (error.response?.status === 401) {
      const username = prompt('Username:')
      const password = prompt('Password:')
      if (username && password) {
        localStorage.setItem('username', username)
        localStorage.setItem('password', password)
        client.defaults.auth = { username, password }
        return client.request(error.config)
      }
    }
    return Promise.reject(error)
  }
)

export default {
  getStatus: () => client.get('/api/status').then(r => r.data),
  getTunnels: () => client.get('/api/tunnels').then(r => r.data),
  getTunnel: (id) => client.get(`/api/tunnels/${id}`).then(r => r.data),
  createTunnel: (data) => client.post('/api/tunnels', data).then(r => r.data),
  deleteTunnel: (id) => client.delete(`/api/tunnels/${id}`).then(r => r.data),
  startTunnel: (id) => client.post(`/api/tunnels/${id}/start`).then(r => r.data),
  stopTunnel: (id) => client.post(`/api/tunnels/${id}/stop`).then(r => r.data),
  getTunnelLogs: (id, limit = 100) => client.get(`/api/tunnels/${id}/logs?limit=${limit}`).then(r => r.data),
  getAllLogs: () => Promise.resolve([]),
  getIngressRules: (tunnelId) => client.get(`/api/ingress?tunnel_id=${tunnelId}`).then(r => r.data),
  createIngressRule: (data) => client.post('/api/ingress', data).then(r => r.data),
  updateIngressRule: (id, data) => client.put(`/api/ingress/${id}`, data).then(r => r.data),
  deleteIngressRule: (id) => client.delete(`/api/ingress/${id}`).then(r => r.data),
  getDomains: (page = 1, perPage = 50) => client.get(`/api/domains?page=${page}&per_page=${perPage}`).then(r => r.data),
  createDNSRecord: (data) => client.post('/api/dns', data).then(r => r.data),
  deleteDNSRecord: (zoneId, recordId) => client.delete(`/api/dns/${zoneId}/${recordId}`).then(r => r.data)
}