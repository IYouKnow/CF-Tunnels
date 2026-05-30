export const APP_SCOPE_OPTIONS = [
  {
    value: 'resources:read',
    label: 'Read app identity/resources',
    description: 'Allows testing /api/v1/me and reading basic app resource info.'
  },
  {
    value: 'dns:read',
    label: 'Read DNS records',
    description: 'Allows reading DNS record status.'
  },
  {
    value: 'dns:create',
    label: 'Create DNS records',
    description: 'Allows creating DNS records through the internal API.'
  },
  {
    value: 'dns:update',
    label: 'Update DNS records',
    description: 'Allows updating existing DNS records, useful for dynamic DNS.'
  }
]

export const APP_SCOPE_PRESETS = {
  readOnly: ['resources:read', 'dns:read'],
  dnsManager: ['resources:read', 'dns:read', 'dns:create', 'dns:update'],
  clear: []
}
