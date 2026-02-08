import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient, ApiError, setUnauthorizedHandler, TOKEN_STORAGE_KEY } from './client'

describe('api client', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    setUnauthorizedHandler(null)
  })

  afterEach(() => {
    setUnauthorizedHandler(null)
  })

  it('maps API error envelopes to ApiError', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'invalid_request', message: 'bad payload' } }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' }
      })
    )

    await expect(apiClient.listApps()).rejects.toMatchObject({
      name: 'ApiError',
      status: 400,
      code: 'invalid_request',
      message: 'bad payload'
    })
  })

  it('clears token and invokes unauthorized handler on 401', async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, 'token-123')
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'unauthorized', message: 'unauthorized' } }), {
        status: 401,
        headers: { 'Content-Type': 'application/json' }
      })
    )

    await expect(apiClient.listApps()).rejects.toBeInstanceOf(ApiError)
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull()
    expect(onUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('fetches auth options without requiring auth', async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, 'team-token')
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ signup_enabled: true, bootstrap_enabled: false }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' }
      })
    )

    const payload = await apiClient.getAuthOptions()
    expect(payload).toEqual({ signup_enabled: true, bootstrap_enabled: false })

    const [, init] = fetchSpy.mock.calls[0]
    const headers = init?.headers as Headers
    expect(headers.get('Authorization')).toBeNull()
  })

  it('submits signup payload without auth header', async () => {
    localStorage.setItem(TOKEN_STORAGE_KEY, 'team-token')
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ team_id: 1, slug: 'acme', token: 'tt_signup', role: 'admin' }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' }
      })
    )

    const payload = await apiClient.signupTeam({ slug: 'acme', name: 'Acme Corp', password: 'secret' })
    expect(payload.slug).toBe('acme')

    const [url, init] = fetchSpy.mock.calls[0]
    expect(url).toBe('/api/v1/teams/signup')
    expect(init?.method).toBe('POST')
    const headers = init?.headers as Headers
    expect(headers.get('Authorization')).toBeNull()
  })
})
