<script setup lang="ts">
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
</script>

<template>
  <aside class="sidebar">
    <div class="brand">
      <div class="brand-icon">
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <rect x="3" y="3" width="18" height="18" rx="4" stroke="currentColor" stroke-width="1.5" fill="none"/>
          <path d="M8 12h8M12 8v8" stroke="url(#brand-grad)" stroke-width="2" stroke-linecap="round"/>
          <defs>
            <linearGradient id="brand-grad" x1="8" y1="8" x2="16" y2="16">
              <stop stop-color="var(--accent-blue)"/>
              <stop offset="1" stop-color="var(--accent-cyan)"/>
            </linearGradient>
          </defs>
        </svg>
      </div>
      <span class="brand-text">MiniTower</span>
    </div>

    <nav class="nav">
      <span class="nav-section">Operations</span>
      <RouterLink to="/home" class="nav-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>
        <span>Home</span>
      </RouterLink>
      <RouterLink to="/apps" class="nav-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
        <span>Apps</span>
      </RouterLink>
      <RouterLink to="/runs" class="nav-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>
        <span>Runs</span>
      </RouterLink>

      <span class="nav-section">Settings</span>
      <RouterLink to="/settings/tokens" class="nav-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0110 0v4"/></svg>
        <span>Tokens</span>
      </RouterLink>
      <RouterLink v-if="auth.isAdmin" to="/admin/runners" class="nav-item">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 14h3M1 9h3M1 14h3"/></svg>
        <span>Runners</span>
      </RouterLink>
    </nav>

    <div class="footer">
      <div class="team-badge">
        <div class="avatar">{{ (auth.teamSlug ?? 'T')[0].toUpperCase() }}</div>
        <div class="team-info">
          <span class="team-name">{{ auth.teamSlug ?? 'Unknown' }}</span>
          <span class="team-role">{{ auth.role ?? 'member' }}</span>
        </div>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 240px;
  height: 100vh;
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  padding: 1.25rem 0.75rem;
  border-right: 1px solid var(--border-default);
  background: var(--bg-secondary);
  overflow-y: auto;
}

.brand {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.25rem 0.65rem 1.25rem;
}

.brand-icon {
  color: var(--accent-blue);
  display: flex;
  align-items: center;
  width: 34px;
  height: 34px;
  justify-content: center;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--accent-blue) 8%, transparent);
  box-shadow: 0 0 12px color-mix(in srgb, var(--accent-blue) 10%, transparent);
}

.brand-text {
  font-size: 1.1rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  background: var(--gradient-brand);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}

.nav-section {
  color: var(--text-tertiary);
  font-size: 0.68rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 1rem 0.65rem 0.35rem;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.5rem 0.65rem;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: 0.875rem;
  font-weight: 500;
  position: relative;
}

.nav-item:hover {
  background: var(--bg-tertiary);
  color: var(--text-primary);
}

.nav-item:hover svg {
  transform: scale(1.08);
}

.nav-item svg {
  transition: transform var(--transition-fast), color var(--transition-fast);
}

.nav-item.router-link-active {
  background: color-mix(in srgb, var(--accent-blue) 10%, var(--bg-tertiary));
  color: var(--text-primary);
  box-shadow:
    inset 2px 0 0 var(--accent-blue),
    0 0 16px color-mix(in srgb, var(--accent-blue) 6%, transparent);
}

.nav-item.router-link-active svg {
  color: var(--accent-blue);
  filter: drop-shadow(0 0 4px color-mix(in srgb, var(--accent-blue) 30%, transparent));
}

.footer {
  margin-top: auto;
  padding-top: 0.75rem;
  border-top: 1px solid var(--border-default);
}

.team-badge {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  padding: 0.4rem 0.5rem;
  border-radius: var(--radius-sm);
  transition: background var(--transition-fast);
}

.team-badge:hover {
  background: var(--bg-tertiary);
}

.avatar {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  background: var(--gradient-brand);
  display: grid;
  place-items: center;
  font-size: 0.75rem;
  font-weight: 700;
  color: white;
  flex-shrink: 0;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-blue) 25%, transparent);
}

.team-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.team-name {
  font-size: 0.82rem;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.team-role {
  font-size: 0.7rem;
  color: var(--text-tertiary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

@media (max-width: 900px) {
  .sidebar {
    width: 100%;
    height: auto;
    position: static;
    flex-direction: row;
    flex-wrap: wrap;
    align-items: center;
    padding: 0.75rem;
    border-right: none;
    border-bottom: 1px solid var(--border-default);
  }

  .brand { padding-bottom: 0; }

  .nav {
    flex-direction: row;
    flex-wrap: wrap;
    gap: 2px;
  }

  .nav-section { display: none; }

  .footer {
    margin-top: 0;
    border-top: none;
    padding-top: 0;
  }
}
</style>
