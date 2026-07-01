<template>
  <router-view />
</template>

<script>
export default {
  name: 'App'
}
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

:root {
  --bg-primary: #070707;
  --bg-secondary: #0c0c0c;
  --bg-tertiary: #111111;
  --bg-elevated: #161616;
  --accent: #f38020;
  --accent-hover: #ff9940;
  --accent-subtle: rgba(243, 128, 32, 0.12);
  --text-primary: #fafafa;
  --text-secondary: #a3a3a3;
  --text-muted: #737373;
  --success: #22c55e;
  --success-subtle: rgba(34, 197, 94, 0.12);
  --error: #ef4444;
  --error-subtle: rgba(239, 68, 68, 0.12);
  --warning: #f59e0b;
  --border: #1f1f1f;
  --border-light: #282828;
  --sidebar-bg: #0c0c0c;
  --topbar-bg: #070707;
  --card-bg: #0c0c0c;
  --radius-sm: 6px;
  --radius-md: 8px;
  --radius-lg: 12px;
  --shadow-sm: 0 1px 2px rgba(0,0,0,0.3);
  --shadow-md: 0 4px 12px rgba(0,0,0,0.3);
  --shadow-lg: 0 8px 24px rgba(0,0,0,0.35);
}

[data-theme="light"] {
  --bg-primary: #fafafa;
  --bg-secondary: #ffffff;
  --bg-tertiary: #f4f4f5;
  --bg-elevated: #ffffff;
  --accent: #f38020;
  --accent-hover: #e67312;
  --accent-subtle: rgba(243, 128, 32, 0.1);
  --text-primary: #18181b;
  --text-secondary: #71717a;
  --text-muted: #a1a1aa;
  --success: #16a34a;
  --success-subtle: rgba(22, 163, 74, 0.1);
  --error: #dc2626;
  --error-subtle: rgba(220, 38, 38, 0.1);
  --warning: #d97706;
  --border: #e4e4e7;
  --border-light: #d4d4d8;
  --sidebar-bg: #ffffff;
  --topbar-bg: #fafafa;
  --card-bg: #ffffff;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--bg-primary);
  color: var(--text-primary);
  min-height: 100vh;
  transition: background 0.3s, color 0.3s;
}

.app {
  display: flex;
  min-height: 100vh;
}

.sidebar {
  width: 240px;
  background: var(--sidebar-bg);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  position: fixed;
  height: 100vh;
  transition: background 0.3s, border-color 0.3s;
}

.sidebar-header {
  padding: 1.25rem;
  border-bottom: 1px solid var(--border);
}

.brand {
  display: flex;
  align-items: center;
  font-size: 1.125rem;
  font-weight: 700;
  letter-spacing: -0.3px;
}

.sidebar-nav {
  flex: 1;
  padding: 0.75rem 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  text-decoration: none;
  font-size: 0.875rem;
  font-weight: 500;
  transition: all 0.15s;
}

.nav-item svg {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.nav-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.nav-item.router-link-exact-active {
  background: var(--accent-subtle);
  color: var(--accent);
}

.nav-item.router-link-exact-active svg {
  stroke: var(--accent);
}

.sidebar-footer {
  padding: 0.75rem;
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.theme-toggle {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.5rem 0.75rem;
  background: transparent;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all 0.15s;
}

.theme-toggle:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.theme-toggle svg {
  width: 18px;
  height: 18px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.375rem 0.5rem;
}

.user-avatar {
  width: 32px;
  height: 32px;
  background: var(--accent);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 600;
  font-size: 0.8rem;
  color: white;
  flex-shrink: 0;
}

.user-name {
  font-size: 0.8125rem;
  font-weight: 500;
}

.main-wrapper {
  flex: 1;
  margin-left: 240px;
  display: flex;
  flex-direction: column;
}

.topbar {
  height: 56px;
  background: var(--topbar-bg);
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.5rem;
  position: sticky;
  top: 0;
  z-index: 10;
  transition: background 0.3s, border-color 0.3s;
}

.page-title {
  font-size: 1rem;
  font-weight: 600;
}

.main-content {
  flex: 1;
  padding: 1.5rem;
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

.btn-action {
  padding: 0.3rem 0.6rem;
  font-size: 0.8rem;
  background: transparent;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s;
  line-height: 1.4;
}

.btn-action:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border-color: var(--border-light);
}

.btn-action.danger {
  color: var(--error);
  border-color: var(--error);
}

.btn-action.danger:hover {
  background: var(--error-subtle);
}

.btn-action.success {
  color: var(--success);
  border-color: var(--success);
}

.btn-action.success:hover {
  background: var(--success-subtle);
}

.form-group {
  margin-bottom: 1rem;
}

.form-group label {
  display: block;
  font-size: 0.8125rem;
  font-weight: 500;
  margin-bottom: 0.375rem;
  color: var(--text-primary);
}

.form-group input,
.form-group select {
  width: 100%;
  padding: 0.5rem 0.75rem;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: border-color 0.15s;
}

.form-group input:focus,
.form-group select:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-subtle);
}

.form-group small {
  display: block;
  margin-top: 0.375rem;
  font-size: 0.8rem;
  color: var(--text-muted);
}

.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  backdrop-filter: blur(4px);
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

.modal h2 {
  font-size: 1.125rem;
  font-weight: 600;
  margin-bottom: 1.25rem;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 1.5rem;
}

.card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}

.card-header {
  padding: 0.875rem 1.25rem;
  border-bottom: 1px solid var(--border);
  font-weight: 600;
  font-size: 0.9rem;
}

.card-body {
  padding: 1rem 1.25rem;
}

.table-header,
.table-row {
  display: grid;
  padding: 0.75rem 1.25rem;
  align-items: center;
}

.table-header {
  background: var(--bg-tertiary);
  font-weight: 500;
  font-size: 0.8rem;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.table-row {
  border-bottom: 1px solid var(--border);
  font-size: 0.875rem;
}

.table-row:last-child {
  border-bottom: none;
}

.table-row:hover {
  background: var(--bg-tertiary);
}

.empty {
  padding: 2.5rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.875rem;
}

.stat-card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: 1.25rem;
  box-shadow: var(--shadow-sm);
}

.stat-value {
  font-size: 2rem;
  font-weight: 700;
  color: var(--text-primary);
}

.stat-label {
  font-size: 0.8125rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
}

.badge {
  display: inline-flex;
  align-items: center;
  padding: 0.2rem 0.625rem;
  border-radius: 9999px;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: capitalize;
  letter-spacing: 0.3px;
}

.badge.running {
  background: var(--success-subtle);
  color: var(--success);
}

.badge.stopped {
  background: var(--bg-tertiary);
  color: var(--text-muted);
}

.badge.error {
  background: var(--error-subtle);
  color: var(--error);
}

.sidebar-overlay {
  display: none;
}

@media (max-width: 768px) {
  .sidebar-overlay {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 150;
    background: rgba(0,0,0,0.5);
  }
  .main-wrapper {
    margin-left: 0;
  }
}
</style>
