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
})
