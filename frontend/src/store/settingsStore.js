import { reactive } from 'vue'

const STORAGE_KEY = 'go-term-settings'
const SETTINGS_VERSION = 1

const defaultSettings = {
  version: SETTINGS_VERSION,
  theme: 'dark', // dark | light
  fontSize: 14,
  fontFamily: 'Consolas, Monaco, "Courier New", monospace',
  cursorStyle: 'bar', // block | bar | underline
  background: '#1e1e1e',
  foreground: '#ffffff',
  selection: 'rgba(65, 105, 225, 0.3)',
  scrollback: 2000,
}

function load() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      // 从旧版本(无 version)升级：旧默认是方块光标 block，统一迁移为细竖线 bar
      if (parsed.version === undefined && parsed.cursorStyle === 'block') {
        parsed.cursorStyle = 'bar'
      }
      return { ...defaultSettings, ...parsed, version: SETTINGS_VERSION }
    }
  } catch (e) {
    /* ignore */
  }
  return { ...defaultSettings }
}

export const settingsStore = reactive(load())

export function saveSettings() {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      version: SETTINGS_VERSION,
      theme: settingsStore.theme,
      fontSize: settingsStore.fontSize,
      fontFamily: settingsStore.fontFamily,
      cursorStyle: settingsStore.cursorStyle,
      background: settingsStore.background,
      foreground: settingsStore.foreground,
      selection: settingsStore.selection,
      scrollback: settingsStore.scrollback,
    })
  )
}

// 生成 xterm 主题对象
export function xtermTheme() {
  return {
    background: settingsStore.background,
    foreground: settingsStore.foreground,
    cursor: settingsStore.foreground,
    selection: settingsStore.selection,
    selectionForeground: settingsStore.background,
  }
}
