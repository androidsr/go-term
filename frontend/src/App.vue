<template>
  <a-config-provider :locale="zhCN" :theme="antTheme" :component-size="antComponentSize">
    <div id="app">
      <ServerManager />
    </div>
  </a-config-provider>
</template>

<script>
import ServerManager from './components/ServerManager.vue';
import zhCN from 'ant-design-vue/es/locale/zh_CN';
import { theme } from 'ant-design-vue';
import { settingsStore, saveSettings } from './store/settingsStore.js';

// 主题对应的终端默认配色（切换主题时自动同步到 xterm，实时生效）
const TERMINAL_PRESETS = {
  dark: {
    background: '#1e1e1e',
    foreground: '#ffffff',
    selection: 'rgba(65, 105, 225, 0.3)',
  },
  light: {
    background: '#f7f8fa',
    foreground: '#24292e',
    selection: 'rgba(9, 105, 218, 0.25)',
  },
};

export default {
  name: 'App',
  components: {
    ServerManager
  },
  data() {
    return {
      zhCN,
      theme,
      settings: settingsStore
    };
  },
  computed: {
    // 主题跟随设置（深色/浅色），settingsStore 是响应式的，切换立即生效
    antTheme() {
      return {
        algorithm: this.settings.theme === 'light' ? theme.defaultAlgorithm : theme.darkAlgorithm
      };
    },
    // 全局组件大小（小/中/大）
    antComponentSize() {
      return this.settings.componentSize || 'small';
    }
  },
  created() {
    // 应用启动时同步页面底色、原生控件配色与全局主题 CSS 变量
    this.applyBodyTheme(settingsStore.theme);
  },
  watch: {
    // 主题切换时：更新页面底色与全局变量，并让终端配色跟随主题（xterm 实时生效，无需重启会话）
    'settings.theme'(v) {
      this.applyBodyTheme(v);
      const preset = TERMINAL_PRESETS[v === 'light' ? 'light' : 'dark'];
      if (preset) {
        settingsStore.background = preset.background;
        settingsStore.foreground = preset.foreground;
        settingsStore.selection = preset.selection;
        saveSettings();
        window.dispatchEvent(new Event('apply-theme'));
      }
    }
  },
  methods: {
    applyBodyTheme(v) {
      const light = v === 'light';
      document.body.style.background = light ? '#ffffff' : '#141414';
      document.body.style.colorScheme = light ? 'light' : 'dark';
      // 全局主题 CSS 变量：各组件的边框、面板底色、日志块配色跟随主题切换
      const root = document.documentElement.style;
      const vars = light ? {
        '--app-border': '#d9d9d9',
        '--app-border-secondary': '#e8e8e8',
        '--app-bg-panel': '#ffffff',
        '--app-bg-code': '#f6f8fa',
        '--app-bg-input': '#ffffff',
        '--app-bg-hover': 'rgba(0, 0, 0, 0.045)',
        '--app-bg-active': 'rgba(22, 119, 255, 0.08)',
        '--app-border-active': '#1677ff',
        '--app-text-primary': '#262626',
        '--app-code-text': '#24292e',
        '--app-text-secondary': '#555555',
        '--app-shadow': '0 2px 8px rgba(0, 0, 0, 0.10)'
      } : {
        '--app-border': '#3a3a3a',
        '--app-border-secondary': '#2e2e2e',
        '--app-bg-panel': '#1f1f1f',
        '--app-bg-code': '#161616',
        '--app-bg-input': '#141414',
        '--app-bg-hover': 'rgba(255, 255, 255, 0.08)',
        '--app-bg-active': 'rgba(22, 119, 255, 0.22)',
        '--app-border-active': '#1677ff',
        '--app-text-primary': '#e8e8e8',
        '--app-code-text': '#e8e8e8',
        '--app-text-secondary': '#bbbbbb',
        '--app-shadow': '0 2px 8px rgba(0, 0, 0, 0.35)'
      };
      Object.keys(vars).forEach((k) => root.setProperty(k, vars[k]));
    }
  }
};
</script>

<style>
#app {
  height: 100vh;
  overflow: hidden;
}

body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
}

/* ===== 弹窗 / 抽屉 / 全局消息跟随主题（teleport 到 body 的组件统一用 CSS 变量覆盖） ===== */
.ant-modal .ant-modal-content,
.ant-modal .ant-modal-header {
  background: var(--app-bg-panel);
}
.ant-modal .ant-modal-title,
.ant-modal .ant-modal-confirm-title {
  color: var(--app-text-primary);
}
.ant-modal .ant-modal-confirm-content {
  color: var(--app-text-secondary);
}
.ant-modal .ant-modal-close {
  color: var(--app-text-secondary);
}
.ant-modal .ant-modal-footer .ant-btn:not(.ant-btn-primary) {
  background: var(--app-bg-input);
  border-color: var(--app-border);
  color: var(--app-text-primary);
}
/* 弹窗内表单控件跟随主题 */
.ant-modal .ant-input,
.ant-modal .ant-input-number,
.ant-modal .ant-input-affix-wrapper,
.ant-modal .ant-select-selector,
.ant-modal textarea.ant-input {
  background: var(--app-bg-input);
  border-color: var(--app-border);
  color: var(--app-text-primary);
}
.ant-modal .ant-select-selection-item,
.ant-modal .ant-select-selection-placeholder {
  color: var(--app-text-primary);
}
.ant-modal .ant-form-item-label > label {
  color: var(--app-text-primary);
}
/* 抽屉（任务中心等）跟随主题 */
.ant-drawer .ant-drawer-content,
.ant-drawer .ant-drawer-header {
  background: var(--app-bg-panel);
}
.ant-drawer .ant-drawer-title,
.ant-drawer .ant-drawer-close {
  color: var(--app-text-primary);
}
/* 全局 message 提示跟随主题 */
.ant-message .ant-message-notice-content {
  background: var(--app-bg-panel);
  color: var(--app-text-primary);
  border: 1px solid var(--app-border);
  box-shadow: var(--app-shadow);
}
/* 下拉/选择浮层跟随主题 */
.ant-select-dropdown {
  background: var(--app-bg-panel);
}
.ant-select-dropdown .ant-select-item {
  color: var(--app-text-primary);
}
.ant-select-dropdown .ant-select-item-option-selected:not(.ant-select-item-option-disabled) {
  background: var(--app-bg-active);
}
.ant-select-dropdown .ant-select-item-option-active:not(.ant-select-item-option-disabled) {
  background: var(--app-bg-hover);
}
</style>
