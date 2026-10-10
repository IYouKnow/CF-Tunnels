// The SPA talks to the Go API same-origin: in production the Go server serves
// both, and in dev the Vite server proxies /api -> :38427 (see vite.config.js).
// Override with VITE_API_BASE_URL only if you serve the API from elsewhere.
export const API_BASE = import.meta.env.VITE_API_BASE_URL || "";

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
