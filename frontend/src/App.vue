<template>
  <a-config-provider :locale="zhCN" :theme="antTheme">
    <div id="app">
      <ServerManager />
    </div>
  </a-config-provider>
</template>

<script>
import ServerManager from './components/ServerManager.vue';
import zhCN from 'ant-design-vue/es/locale/zh_CN';
import { theme } from 'ant-design-vue';
import { settingsStore } from './store/settingsStore.js';

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
    }
  },
  watch: {
    'settings.theme': {
      immediate: true,
      handler(v) {
        // 同步页面底色与浏览器原生控件配色
        const light = v === 'light';
        document.body.style.background = light ? '#f5f5f5' : '#141414';
        document.body.style.colorScheme = light ? 'light' : 'dark';
      }
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
</style>
