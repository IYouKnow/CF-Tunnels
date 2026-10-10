// CF Tunnels — data access layer.
//
// Single seam between the UI and the Go/Gin backend. Every method is async and
// throws ApiError on failure, so components can rely on try/catch.

import { request, API_BASE } from "./http";

// Some Go handlers return a bare slice that marshals to JSON `null` when empty
// (e.g. /api/ingress, /api/logs); normalize those to an array so callers can
// always map over the result.
const asArray = (v) => (Array.isArray(v) ? v : []);

function normalizeTunnel(t, zonesById) {
  const zoneName = t.zone_id != null ? zonesById[t.zone_id] ?? null : null;
  return {
    id: t.id,
    name: t.name,
    uuid: t.uuid,
    status: t.status || "stopped",
    pid: t.pid || null,
    subdomain: t.subdomain || "",
    domain: t.domain || zoneName || "",
    zoneId: t.zone_id ?? null,
    zoneName,
    publicHostname: t.subdomain && t.domain ? `${t.subdomain}.${t.domain}` : "",
    localService: t.address || "",
    autoStart: !!t.auto_start,
    extraDomainCount: t.extra_domains || 0,
    createdAt: t.created_at,
  };
}

function normalizeRecord(r) {
  return {
    id: r.id,
    type: r.type,
    name: r.name,
    content: r.content,
    ttl: r.ttl,
    proxied: !!r.proxied,
  };
}

async function fetchZonesById() {
  try {
    const data = await request("/api/domains", { query: { per_page: 200 } });
    return Object.fromEntries(asArray(data?.domains).map((z) => [z.id, z.name]));
  } catch {
    return {};
  }
}

export const api = {
  auth: {
    configStatus: () => request("/api/auth/config-status", { skipAuthRedirect: true }),
    me: () => request("/api/auth/me", { skipAuthRedirect: true }),
    login: (username, password) =>
      request("/api/login", { method: "POST", body: { username, password }, skipAuthRedirect: true }),
    logout: () => request("/api/logout", { method: "POST" }),
  },

  tunnels: {
    async list() {
      const [list, zonesById] = await Promise.all([
        request("/api/tunnels", { query: { per_page: 200 } }),
        fetchZonesById(),
      ]);
      return asArray(list?.tunnels).map((t) => normalizeTunnel(t, zonesById));
    },
    get: (id) => request(`/api/tunnels/${id}`),
    create: (body) => request("/api/tunnels", { method: "POST", body }),
    update: (id, body) => request(`/api/tunnels/${id}`, { method: "PUT", body }),
    remove: (id) => request(`/api/tunnels/${id}`, { method: "DELETE" }),
    start: (id) => request(`/api/tunnels/${id}/start`, { method: "POST" }),
    stop: (id) => request(`/api/tunnels/${id}/stop`, { method: "POST" }),
    sync: () => request("/api/tunnels/sync", { method: "POST" }),
  },

  ingress: {
    list: async (tunnelId) => asArray(await request("/api/ingress", { query: { tunnel_id: tunnelId } })),
    create: (body) => request("/api/ingress", { method: "POST", body }),
    update: (id, body) => request(`/api/ingress/${id}`, { method: "PUT", body }),
    remove: (id) => request(`/api/ingress/${id}`, { method: "DELETE" }),
  },

  dns: {
    zones: async () => {
      const data = await request("/api/domains", { query: { per_page: 200 } });
      return asArray(data?.domains).map((z) => ({ id: z.id, name: z.name }));
    },
    async records(zoneId, { page = 1, perPage = 200 } = {}) {
      const data = await request("/api/dns/records", {
        query: { zone_id: zoneId, page, per_page: perPage },
      });
      return { records: asArray(data?.records).map(normalizeRecord), total: data?.total || 0 };
    },
    create: (body) => request("/api/dns", { method: "POST", body }),
    update: (zoneId, recordId, body) => request(`/api/dns/${zoneId}/${recordId}`, { method: "PUT", body }),
    remove: (zoneId, recordId) => request(`/api/dns/${zoneId}/${recordId}`, { method: "DELETE" }),
    // Counts records across every zone for the dashboard stat card.
    async stats() {
      const zones = await api.dns.zones();
      const totals = await Promise.all(
        zones.map((z) => api.dns.records(z.id).then((r) => r.total).catch(() => 0))
      );
      return { zones: zones.length, records: totals.reduce((a, b) => a + b, 0) };
    },
  },

  logs: {
    async list(limit = 500) {
      const data = await request("/api/logs", { query: { limit } });
      return asArray(data).map((l) => ({
        id: l.id,
        tunnelId: l.tunnel_id,
        timestamp: l.timestamp,
        severity: l.level,
        message: l.message,
      }));
    },
    streamUrl: (tunnelId) => {
      const url = `${API_BASE}/api/logs/stream`;
      return tunnelId ? `${url}?tunnel_id=${tunnelId}` : url;
    },
  },

  apps: {
    list: async () => asArray(await request("/api/apps")),
    get: (id) => request(`/api/apps/${id}`),
    create: (body) => request("/api/apps", { method: "POST", body }),
    remove: (id) => request(`/api/apps/${id}`, { method: "DELETE" }),
    tokens: async (id) => asArray(await request(`/api/apps/${id}/tokens`)),
    createToken: (id, body) => request(`/api/apps/${id}/tokens`, { method: "POST", body }),
    revokeToken: (id, tokenId) => request(`/api/apps/${id}/tokens/${tokenId}`, { method: "DELETE" }),
    purgeToken: (id, tokenId) =>
      request(`/api/apps/${id}/tokens/${tokenId}/purge`, { method: "DELETE" }),
  },

  tokens: {
    async list() {
      const apps = await api.apps.list();
      const groups = await Promise.all(
        apps.map((app) =>
          api.apps
            .tokens(app.id)
            .then((tokens) => tokens.map((t) => ({ ...t, app })))
            .catch(() => [])
        )
      );
      return groups.flat().map((t) => ({
        id: t.id,
        appId: t.app.id,
        appName: t.app.name,
        name: t.name,
        prefix: t.tokenPrefix,
        scopes: t.scopes || [],
        created: t.createdAt,
        lastUsed: t.lastUsedAt,
        revoked: !!t.revokedAt,
        status: t.revokedAt ? "revoked" : "active",
      }));
    },
  },

  system: {
    status: () => request("/api/status"),
    versions: async () => {
      const [app, cloudflared] = await Promise.all([
        request("/api/app/version").catch(() => ({ version: "unknown", docker: false })),
        request("/api/cloudflared/version").catch(() => ({ version: "unknown" })),
      ]);
      return {
        appVersion: app.version,
        docker: !!app.docker,
        cloudflaredVersion: cloudflared.version,
      };
    },
    checkAppUpdate: () => request("/api/app/check-update"),
    updateApp: () => request("/api/app/update", { method: "POST" }),
    checkCloudflaredUpdate: () => request("/api/cloudflared/check-update"),
    updateCloudflared: () => request("/api/cloudflared/update", { method: "POST" }),
    async cloudflare() {
      try {
        await request("/api/domains", { query: { per_page: 1 } });
        return { connected: true };
      } catch (err) {
        return { connected: false, error: err.message };
      }
    },
  },
};
