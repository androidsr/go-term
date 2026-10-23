import { reactive } from 'vue'

const STORAGE_KEY = 'go-term-settings'
const SETTINGS_VERSION = 2

const defaultSettings = {
  version: SETTINGS_VERSION,
  theme: 'dark', // dark | light
  componentSize: 'middle', // small | middle | large（全局组件大小：小/中/大，列表/表格跟随此设置）
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
      // v1 → v2：组件大小此前只有旧默认值 small（非用户主动选择），统一迁移为正常的 middle
      if (parsed.version === 1 && (!parsed.componentSize || parsed.componentSize === 'small')) {
        parsed.componentSize = 'middle'
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
      componentSize: settingsStore.componentSize,
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
