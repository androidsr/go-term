import { reactive } from 'vue';

function uid(prefix) {
  return prefix + '_' + Date.now() + '_' + Math.random().toString(36).slice(2, 8);
}

// 全局任务中心：统一管理文件传输任务与批量脚本任务
export const taskStore = reactive({
  drawerVisible: false,
  activeTab: 'transfer',
  transfers: [], // 文件上传/下载任务
  scripts: [],   // 批量脚本执行任务
  unread: 0,

  openDrawer(tab) {
    if (tab) this.activeTab = tab;
    this.drawerVisible = true;
    this.unread = 0;
  },

  // ========== 传输任务 ==========
  addTransfer(t) {
    const task = Object.assign(
      {
        id: uid('tr'),
        type: 'upload', // upload | download
        serverID: '',
        serverName: '',
        name: '',
        percent: 0,
        transferred: 0,
        total: 0,
        speed: '0 B/s',
        status: 'running', // running | success | error
        error: ''
      },
      t
    );
    this.transfers.unshift(task);
    this.unread++;
    return task.id;
  },

  updateTransfer(id, patch) {
    const t = this.transfers.find((x) => x.id === id);
    if (t) Object.assign(t, patch);
  },

  removeTransfer(id) {
    this.transfers = this.transfers.filter((x) => x.id !== id);
  },

  clearFinishedTransfers() {
    this.transfers = this.transfers.filter((x) => x.status === 'running');
  },

  // ========== 脚本任务 ==========
  addScriptTask(t) {
    const task = Object.assign(
      {
        id: uid('sc'),
        scriptID: '',
        name: '',
        status: 'running', // running | success | failed
        servers: {}, // serverID -> { serverName, status, commandOutputs:[], error }
        total: 0,
        done: 0,
        createdAt: new Date().toLocaleString('zh-CN'),
        error: ''
      },
      t
    );
    this.scripts.unshift(task);
    this.unread++;
    return task.id;
  },

  setScriptServers(id, serverList) {
    const t = this.scripts.find((x) => x.id === id);
    if (t) {
      t.total = serverList.length;
      serverList.forEach((sid) => {
        if (!t.servers[sid]) {
          t.servers[sid] = {
            serverID: sid,
            serverName: '',
            status: 'running',
            commandOutputs: [],
            error: ''
          };
        }
      });
    }
  },

  updateScriptServer(id, serverID, patch) {
    const t = this.scripts.find((x) => x.id === id);
    if (!t) return;
    if (!t.servers[serverID]) {
      t.servers[serverID] = {
        serverID,
        serverName: '',
        status: 'running',
        commandOutputs: [],
        error: ''
      };
    }
    Object.assign(t.servers[serverID], patch);
    // 统计完成数
    const vals = Object.values(t.servers);
    t.done = vals.filter((s) => s.status !== 'running').length;
    if (t.done >= t.total && t.total > 0) {
      const anyFailed = vals.some((s) => s.status === 'failed');
      t.status = anyFailed ? 'failed' : 'success';
    }
  },

  updateScriptTask(id, patch) {
    const t = this.scripts.find((x) => x.id === id);
    if (t) Object.assign(t, patch);
  },

  removeScriptTask(id) {
    this.scripts = this.scripts.filter((x) => x.id !== id);
  }
});

export function newUID(prefix) {
  return uid(prefix);
}
