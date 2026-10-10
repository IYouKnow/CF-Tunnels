// Thin fetch wrapper around the Go/Gin backend.
//
// In dev the Vite server proxies /api -> :38427 (see vite.config.js). When the
// SPA is served by the Go server itself it is same-origin. If someone opens the
// dev server on a non-standard host we fall back to hitting the API port
// directly, mirroring the previous Vue client's behaviour.
const inferredDevBase =
  typeof window !== "undefined" && window.location.port && window.location.port !== "38427"
    ? `${window.location.protocol}//${window.location.hostname}:38427`
    : "";

export const API_BASE = import.meta.env.VITE_API_BASE_URL || inferredDevBase;

export class ApiError extends Error {
  constructor(message, status, data) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.data = data;
  }
}

function buildUrl(path, query) {
  let url = `${API_BASE}${path}`;
  if (query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === "") continue;
      params.append(key, value);
    }
    const qs = params.toString();
    if (qs) url += `?${qs}`;
  }
  return url;
}

export async function request(path, { method = "GET", body, query, headers, skipAuthRedirect } = {}) {
  const hasBody = body !== undefined;
  const res = await fetch(buildUrl(path, query), {
    method,
    credentials: "include",
    headers: {
      ...(hasBody ? { "Content-Type": "application/json" } : {}),
      ...headers,
    },
    body: hasBody ? JSON.stringify(body) : undefined,
  });

  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }

  if (!res.ok) {
    if (res.status === 401 && !skipAuthRedirect) {
      window.dispatchEvent(new CustomEvent("cft:unauthorized"));
    }
    const message =
      (data && typeof data === "object" && data.error) ||
      (typeof data === "string" && data.trim()) ||
      `Request failed (${res.status})`;
    throw new ApiError(message, res.status, data);
  }

  return data;
}
