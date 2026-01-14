<template>
  <a-config-provider :locale="zhCN" :theme="{
    algorithm: theme.darkAlgorithm
  }">
    <div id="app">
      <ServerManager />
    </div>
  </a-config-provider>
</template>

<script>
import ServerManager from './components/ServerManager.vue';
import zhCN from 'ant-design-vue/es/locale/zh_CN';
import { theme } from 'ant-design-vue';

export default {
  name: 'App',
  components: {
    ServerManager
  },
  data() {
    return {
      zhCN,
      theme
    };
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
