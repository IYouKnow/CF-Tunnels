// Scopes accepted by the app-token API (see backend internal/apps/scopes.go).
export const allScopes = ["resources:read", "dns:read", "dns:create", "dns:update"];

export const scopeDescriptions = {
  "resources:read": "Read app identity and resources",
  "dns:read": "Read DNS records",
  "dns:create": "Create DNS records",
  "dns:update": "Update DNS records",
};
