import { computed, ref, watch } from 'vue'

export type Theme = 'dark' | 'light'

const THEME_STORAGE_KEY = 'minitower_theme'

function getInitialTheme(): Theme {
  const stored = localStorage.getItem(THEME_STORAGE_KEY)
  if (stored === 'dark' || stored === 'light') {
    return stored
  }
  return 'dark'
}

function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute('data-theme', theme)
}

const theme = ref<Theme>(getInitialTheme())
let initialized = false

export function useTheme() {
  if (!initialized) {
    applyTheme(theme.value)
    watch(
      theme,
      (value) => {
        applyTheme(value)
        localStorage.setItem(THEME_STORAGE_KEY, value)
      },
      { immediate: false }
    )
    initialized = true
  }

  const isLight = computed(() => theme.value === 'light')

  function setTheme(value: Theme): void {
    theme.value = value
  }

  function toggleTheme(): void {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
  }

  return {
    theme,
    isLight,
    setTheme,
    toggleTheme
  }
}
