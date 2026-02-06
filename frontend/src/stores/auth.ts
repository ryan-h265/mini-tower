import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { apiClient, clearStoredToken, TOKEN_STORAGE_KEY } from '../api/client'
import type { BootstrapTeamRequest, TokenRole } from '../api/types'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem(TOKEN_STORAGE_KEY))
  const teamId = ref<number | null>(null)
  const teamSlug = ref<string | null>(null)
  const tokenId = ref<number | null>(null)
  const role = ref<TokenRole | null>(null)

  const isAuthenticated = computed(() => Boolean(token.value))
  const isAdmin = computed(() => role.value === 'admin')

  function setToken(nextToken: string | null): void {
    token.value = nextToken
    if (!nextToken) {
      clearStoredToken()
      return
    }
    localStorage.setItem(TOKEN_STORAGE_KEY, nextToken)
  }

  async function fetchMe(): Promise<void> {
    const me = await apiClient.getMe()
    teamId.value = me.team_id
    teamSlug.value = me.team_slug
    tokenId.value = me.token_id
    role.value = me.role
  }

  async function login(slug: string, password: string): Promise<void> {
    const response = await apiClient.loginTeam({ slug, password })
    setToken(response.token)
    tokenId.value = response.token_id
    role.value = response.role
    teamId.value = response.team_id

    try {
      await fetchMe()
    } catch (error) {
      logout()
      throw error
    }
  }

  async function bootstrap(payload: BootstrapTeamRequest, bootstrapToken: string): Promise<void> {
    const response = await apiClient.bootstrapTeam(payload, bootstrapToken)
    setToken(response.token)
    role.value = response.role
    teamId.value = response.team_id
    teamSlug.value = response.slug

    try {
      await fetchMe()
    } catch (error) {
      logout()
      throw error
    }
  }

  function logout(): void {
    setToken(null)
    teamId.value = null
    teamSlug.value = null
    tokenId.value = null
    role.value = null
  }

  async function rehydrate(): Promise<void> {
    if (!token.value) {
      return
    }

    try {
      await fetchMe()
    } catch {
      logout()
    }
  }

  return {
    token,
    teamId,
    teamSlug,
    tokenId,
    role,
    isAuthenticated,
    isAdmin,
    login,
    bootstrap,
    fetchMe,
    logout,
    rehydrate
  }
})
