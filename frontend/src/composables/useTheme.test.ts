import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

describe('useTheme', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
  })

  it('defaults to dark and persists toggle changes', async () => {
    const { useTheme } = await import('./useTheme')
    const themeState = useTheme()

    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(themeState.isLight.value).toBe(false)

    themeState.toggleTheme()
    await nextTick()

    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(localStorage.getItem('minitower_theme')).toBe('light')
    expect(themeState.isLight.value).toBe(true)
  })

  it('rehydrates stored theme preference', async () => {
    localStorage.setItem('minitower_theme', 'light')
    const { useTheme } = await import('./useTheme')
    const themeState = useTheme()

    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(themeState.theme.value).toBe('light')
    expect(themeState.isLight.value).toBe(true)
  })
})
