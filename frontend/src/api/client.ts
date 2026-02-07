import type {
  AppResponse,
  AdminRunnersResponse,
  BootstrapTeamRequest,
  BootstrapTeamResponse,
  CreateAppRequest,
  CreateRunRequest,
  CreateTokenRequest,
  CreateTokenResponse,
  CreateVersionRequest,
  ErrorEnvelope,
  ListAppsResponse,
  ListRunsResponse,
  ListVersionsResponse,
  LoginRequest,
  LoginResponse,
  MeResponse,
  RunLogsResponse,
  RunResponse,
  RunsSummaryResponse,
  VersionResponse
} from './types'

export const TOKEN_STORAGE_KEY = 'minitower_token'

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

interface RequestOptions {
  method?: HttpMethod
  query?: Record<string, string | number | undefined>
  body?: unknown
  headers?: Record<string, string>
  auth?: boolean
  tokenOverride?: string
}

export class ApiError extends Error {
  public readonly status: number
  public readonly code?: string

  constructor(message: string, status: number, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler
}

export function readStoredToken(): string | null {
  return localStorage.getItem(TOKEN_STORAGE_KEY)
}

export function clearStoredToken(): void {
  localStorage.removeItem(TOKEN_STORAGE_KEY)
}

function buildUrl(path: string, query?: Record<string, string | number | undefined>): string {
  const base = ((import.meta.env.VITE_API_BASE_URL as string | undefined) ?? '').trim().replace(/\/+$/, '')
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  const params = new URLSearchParams()
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === '') {
        continue
      }
      params.set(key, String(value))
    }
  }
  const suffix = params.toString()
  const withQuery = suffix ? `${normalizedPath}?${suffix}` : normalizedPath
  return base ? `${base}${withQuery}` : withQuery
}

async function parseError(response: Response): Promise<ApiError> {
  let envelope: ErrorEnvelope | null = null

  try {
    envelope = (await response.json()) as ErrorEnvelope
  } catch {
    envelope = null
  }

  const message = envelope?.error?.message ?? `Request failed with status ${response.status}`
  const code = envelope?.error?.code
  return new ApiError(message, response.status, code)
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const {
    method = 'GET',
    query,
    body,
    headers = {},
    auth = true,
    tokenOverride
  } = options

  const requestHeaders = new Headers(headers)

  if (auth) {
    const token = tokenOverride ?? readStoredToken()
    if (token) {
      requestHeaders.set('Authorization', `Bearer ${token}`)
    }
  }

  let requestBody: BodyInit | undefined
  if (body !== undefined && body !== null) {
    if (body instanceof FormData) {
      requestBody = body
    } else {
      requestHeaders.set('Content-Type', 'application/json')
      requestBody = JSON.stringify(body)
    }
  }

  const response = await fetch(buildUrl(path, query), {
    method,
    headers: requestHeaders,
    body: requestBody
  })

  if (response.status === 401 && auth) {
    clearStoredToken()
    unauthorizedHandler?.()
  }

  if (!response.ok) {
    throw await parseError(response)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return (await response.json()) as T
}

export const apiClient = {
  loginTeam(payload: LoginRequest): Promise<LoginResponse> {
    return request<LoginResponse>('/api/v1/teams/login', {
      method: 'POST',
      body: payload,
      auth: false
    })
  },

  bootstrapTeam(payload: BootstrapTeamRequest, bootstrapToken: string): Promise<BootstrapTeamResponse> {
    return request<BootstrapTeamResponse>('/api/v1/bootstrap/team', {
      method: 'POST',
      body: payload,
      auth: true,
      tokenOverride: bootstrapToken
    })
  },

  getMe(): Promise<MeResponse> {
    return request<MeResponse>('/api/v1/me')
  },

  listApps(): Promise<ListAppsResponse> {
    return request<ListAppsResponse>('/api/v1/apps')
  },

  getApp(appSlug: string): Promise<AppResponse> {
    return request<AppResponse>(`/api/v1/apps/${encodeURIComponent(appSlug)}`)
  },

  createApp(payload: CreateAppRequest): Promise<AppResponse> {
    return request<AppResponse>('/api/v1/apps', {
      method: 'POST',
      body: payload
    })
  },

  listVersions(appSlug: string): Promise<ListVersionsResponse> {
    return request<ListVersionsResponse>(`/api/v1/apps/${encodeURIComponent(appSlug)}/versions`)
  },

  createVersion(appSlug: string, payload: CreateVersionRequest): Promise<VersionResponse> {
    const formData = new FormData()
    formData.set('artifact', payload.artifact)

    return request<VersionResponse>(`/api/v1/apps/${encodeURIComponent(appSlug)}/versions`, {
      method: 'POST',
      body: formData
    })
  },

  listRunsByApp(
    appSlug: string,
    params: {
      limit?: number
      offset?: number
    } = {}
  ): Promise<ListRunsResponse> {
    return request<ListRunsResponse>(`/api/v1/apps/${encodeURIComponent(appSlug)}/runs`, {
      query: {
        limit: params.limit,
        offset: params.offset
      }
    })
  },

  createRun(appSlug: string, payload: CreateRunRequest): Promise<RunResponse> {
    return request<RunResponse>(`/api/v1/apps/${encodeURIComponent(appSlug)}/runs`, {
      method: 'POST',
      body: payload
    })
  },

  listRunsByTeam(params: {
    limit?: number
    offset?: number
    status?: string
    app?: string
  } = {}): Promise<ListRunsResponse> {
    return request<ListRunsResponse>('/api/v1/runs', {
      query: {
        limit: params.limit,
        offset: params.offset,
        status: params.status,
        app: params.app
      }
    })
  },

  getRunsSummary(): Promise<RunsSummaryResponse> {
    return request<RunsSummaryResponse>('/api/v1/runs/summary')
  },

  getRun(runId: number | string): Promise<RunResponse> {
    return request<RunResponse>(`/api/v1/runs/${runId}`)
  },

  getRunLogs(runId: number | string, afterSeq = 0): Promise<RunLogsResponse> {
    return request<RunLogsResponse>(`/api/v1/runs/${runId}/logs`, {
      query: { after_seq: afterSeq }
    })
  },

  cancelRun(runId: number | string): Promise<RunResponse> {
    return request<RunResponse>(`/api/v1/runs/${runId}/cancel`, {
      method: 'POST'
    })
  },

  listAdminRunners(): Promise<AdminRunnersResponse> {
    return request<AdminRunnersResponse>('/api/v1/admin/runners')
  },

  createToken(payload: CreateTokenRequest): Promise<CreateTokenResponse> {
    return request<CreateTokenResponse>('/api/v1/tokens', {
      method: 'POST',
      body: payload
    })
  }
}
