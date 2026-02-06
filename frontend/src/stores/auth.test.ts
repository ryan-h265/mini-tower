import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'
import { apiClient, TOKEN_STORAGE_KEY } from '../api/client'

describe('auth store', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    setActivePinia(createPinia())
  })

  it('logs in and hydrates identity via /me', async () => {
    vi.spyOn(apiClient, 'loginTeam').mockResolvedValue({
      team_id: 7,
      token: 'team-token',
      token_id: 101,
      role: 'admin'
    })
    vi.spyOn(apiClient, 'getMe').mockResolvedValue({
      team_id: 7,
      team_slug: 'acme',
      token_id: 101,
      role: 'admin'
    })

    const store = useAuthStore()
    await store.login('acme', 'secret')

    expect(store.isAuthenticated).toBe(true)
    expect(store.isAdmin).toBe(true)
    expect(store.teamSlug).toBe('acme')
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBe('team-token')
  })

  it('rolls back auth state when /me fails after login', async () => {
    vi.spyOn(apiClient, 'loginTeam').mockResolvedValue({
      team_id: 9,
      token: 'temp-token',
      token_id: 55,
      role: 'admin'
    })
    vi.spyOn(apiClient, 'getMe').mockRejectedValue(new Error('unauthorized'))

    const store = useAuthStore()
    await expect(store.login('broken', 'secret')).rejects.toThrow('unauthorized')

    expect(store.isAuthenticated).toBe(false)
    expect(store.token).toBeNull()
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull()
  })

  it('rehydrate clears stale token when /me fails', async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, 'stale-token')
    vi.spyOn(apiClient, 'getMe').mockRejectedValue(new Error('expired token'))

    const store = useAuthStore()
    await store.rehydrate()

    expect(store.isAuthenticated).toBe(false)
    expect(store.token).toBeNull()
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull()
  })
})
