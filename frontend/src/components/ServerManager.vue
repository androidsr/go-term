<template>
  <div>
    <div class="main-tabs-container">
      <a-tabs v-model:activeKey="activeKey" size="small" :hideAdd="true" type="editable-card"
        @edit="closeTerminalTab" @change="onTabChange" @tabClick="onTabClick">
        <template #tabBarExtraContent>
          <a-space>
            <a-button size="small" @click="cloneActiveSession">
              <CopyOutlined />复制会话
            </a-button>
            <a-badge :count="taskStore.unread" :offset="[-4, 2]">
              <a-button size="small" @click="taskStore.openDrawer()">
                <UnorderedListOutlined />任务中心
              </a-button>
            </a-badge>
          </a-space>
        </template>

        <!-- 主页标签页 - 服务器管理 -->
        <a-tab-pane key="home" tab="主页" :closable="false" force-render>
          <a-layout class="layout" style="background: transparent;">
            <a-layout-sider width="270" class="sider" style="background: transparent;">
              <div class="group-section">
                <div class="group-header">
                  <h3>服务器分组</h3>
                  <a-button type="primary" size="small" @click="showAddGroupModal">+</a-button>
                </div>
                <a-menu :selectedKeys="selectedGroupKeys" mode="inline" @select="onGroupSelect">
                  <a-menu-item v-for="group in groups" :key="group.id">
                    <template #icon>
                      <FolderOutlined />
                    </template>
                    {{ group.name }}
                    <span class="group-actions">
                      <EditOutlined @click.stop="editGroup(group)" />
                      <DeleteOutlined @click.stop="deleteGroup(group.id)" />
                    </span>
                  </a-menu-item>
                </a-menu>
              </div>
            </a-layout-sider>

            <a-layout style="background: transparent;">
              <a-layout-content class="content" style="background: transparent;">
                <div class="server-header">
                  <a-button v-if="currentGroupId" type="primary" @click="showAddServerModal">
                    <PlusOutlined /> 添加服务器
                  </a-button>
                </div>

                <a-table :dataSource="currentServers" :columns="serverColumns" :pagination="false" rowKey="id" size="small">
                  <template #bodyCell="{ column, record }">
                    <template v-if="column.dataIndex === 'status'">
                      <a-tag :color="record.connected ? 'green' : 'red'">
                        {{ record.connected ? '已连接' : '未连接' }}
                      </a-tag>
                    </template>
                    <template v-else-if="column.dataIndex === 'action'">
                      <a-space>
                        <a-button size="small" :type="record.connected ? 'default' : 'primary'"
                          @click="connectServer(record)" :loading="record.loading">
                          <WifiOutlined />{{ record.connected ? '断开' : '连接' }}
                        </a-button>
                        <a-button size="small" :disabled="!record.connected" @click="openTerminal(record)">
                          <CodeOutlined />终端
                        </a-button>
                        <a-button size="small" :disabled="!record.connected" @click="manageFiles(record)">
                          <FolderOutlined />文件
                        </a-button>
                        <a-button size="small" :disabled="record.connected" @click="editServer(record)">
                          <EditOutlined />编辑
                        </a-button>
                        <a-button size="small" :disabled="record.connected" @click="deleteServer(record)">
                          <DeleteOutlined />删除
                        </a-button>
                      </a-space>
                    </template>
                  </template>
                </a-table>
              </a-layout-content>
            </a-layout>
          </a-layout>
        </a-tab-pane>

        <!-- 批量脚本标签页 -->
        <a-tab-pane key="batch-script" tab="预设脚本" :closable="false" force-render>
          <BatchScriptManager />
        </a-tab-pane>

        <!-- 运维配置标签页 -->
        <a-tab-pane key="ops-config" tab="运维配置" :closable="false" force-render>
          <OpsConfigManager />
        </a-tab-pane>

        <!-- 终端 / 文件 标签页（支持同一服务器多个会话） -->
        <a-tab-pane v-for="tab in terminalTabs" :key="tab.id" :tab="tab.title" closable force-render>
          <div v-if="tab.type === 'terminal'" style="height: 100%; padding: 0; margin: 0">
            <Terminal :server="tab.server" :server-id="tab.serverId" :session-id="tab.sessionId"
              :active="activeKey === tab.id" @terminal-ready="checkPendingScript" />
          </div>
          <div v-else-if="tab.type === 'file'" style="height: 100%; padding: 0; margin: 0">
            <FileManager :server="tab.server" :server-id="tab.serverId" />
          </div>
        </a-tab-pane>
      </a-tabs>
    </div>

    <!-- 添加/编辑分组模态框 -->
    <a-modal v-model:open="groupModalVisible" :title="editingGroup ? '编辑分组' : '添加分组'" @ok="handleGroupModalOk">
      <a-form :model="groupForm" layout="vertical">
        <a-form-item label="分组名称" required>
          <a-input v-model:value="groupForm.name" placeholder="请输入分组名称" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 添加/编辑服务器模态框 -->
    <a-modal v-model:open="serverModalVisible" :title="editingServer ? '编辑服务器' : '添加服务器'" @ok="handleServerModalOk">
      <a-form :model="serverForm" :labelCol="{ span: 5 }">
        <a-form-item label="服务器名称" required>
          <a-input v-model:value="serverForm.name" placeholder="请输入服务器名称" />
        </a-form-item>
        <a-form-item label="主机地址" required>
          <a-input v-model:value="serverForm.host" placeholder="请输入主机地址" />
        </a-form-item>
        <a-form-item label="端口" required>
          <a-input-number v-model:value="serverForm.port" :min="1" :max="65535" style="width: 100%" />
        </a-form-item>
        <a-form-item label="用户名" required>
          <a-input v-model:value="serverForm.username" placeholder="请输入用户名" />
        </a-form-item>
        <a-form-item label="认证方式">
          <a-radio-group v-model:value="authMethod">
            <a-radio value="password">密码</a-radio>
            <a-radio value="key">密钥</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item v-if="authMethod === 'password'" label="密码">
          <a-input-password v-model:value="serverForm.password" placeholder="请输入密码" />
        </a-form-item>
        <a-form-item v-if="authMethod === 'key'" label="私钥文件路径">
          <a-input v-model:value="serverForm.keyFile" placeholder="请输入私钥文件路径" />
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="serverForm.note" placeholder="请输入备注信息（可选）" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 本地命令输出弹窗 -->
    <a-modal v-model:open="localCommandOutputVisible" title="本地命令输出" width="800px" :footer="null"
      :bodyStyle="{ maxHeight: '60vh', overflow: 'hidden' }">
      <div class="local-command-output">
        <div class="output-container">
          <div v-for="(item, index) in localCommandOutputs" :key="index" class="command-item">
            <div class="command-header">
              <span class="command-number">[{{ index + 1 }}]</span>
              <strong>命令:</strong>
              <code>{{ item.command }}</code>
            </div>
            <div class="output-header">
              <strong>输出:</strong>
            </div>
            <pre class="output-content">{{ item.output }}</pre>
          </div>
        </div>
      </div>
    </a-modal>

    <!-- 任务中心 -->
    <TaskCenter />
  </div>
</template>

<script>
import {
  AddServer,
  AddServerGroup,
  CloseTerminalSession,
  ConnectToServer,
  DeleteServer,
  DeleteServerGroup,
  DisconnectFromServer,
  GetServerGroups,
  GetServerConnectionStatus,
  UpdateServer,
  UpdateServerGroup,
  CreateTerminalSessionWithSize,
  SendScriptToTerminal
} from '../../bindings/go-term/controllers/sshcontroller'
import { Events } from '@wailsio/runtime'
import Terminal from './Terminal.vue'
import FileManager from './FileManager.vue'
import BatchScriptManager from './BatchScriptManager.vue'
import OpsConfigManager from './OpsConfigManager.vue'
import TaskCenter from './TaskCenter.vue'
import { taskStore } from '../store/taskStore.js'
import {
  FolderOutlined,
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  WifiOutlined,
  CodeOutlined,
  CopyOutlined,
  UnorderedListOutlined
} from '@ant-design/icons-vue'

export default {
  name: 'ServerManager',
  components: {
    Terminal,
    FileManager,
    BatchScriptManager,
    OpsConfigManager,
    TaskCenter,
    FolderOutlined,
    EditOutlined,
    DeleteOutlined,
    PlusOutlined,
    WifiOutlined,
    CodeOutlined,
    CopyOutlined,
    UnorderedListOutlined
  },
  data() {
    return {
      loading: false,
      groups: [],
      selectedGroupKeys: [],
      currentGroupId: '',
      currentGroupName: '请选择分组',
      currentServers: [],
      activeKey: 'home',
      terminalTabs: [],
      closedSessions: new Set(),
      pendingScript: null,

      localCommandOutputVisible: false,
      localCommandOutputs: [],

      groupModalVisible: false,
      editingGroup: null,
      groupForm: { name: '' },

      serverModalVisible: false,
      editingServer: null,
      authMethod: 'password',
      serverForm: {
        name: '', host: '', port: 22, username: 'root', password: '', keyFile: '', note: ''
      },

      serverColumns: [
        { title: '服务器名称', dataIndex: 'name', key: 'name' },
        { title: '主机地址', dataIndex: 'host', key: 'host' },
        { title: '端口', dataIndex: 'port', key: 'port' },
        { title: '状态', dataIndex: 'status', key: 'status' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    }
  },
  computed: {
    taskStore() {
      return taskStore
    }
  },
  async mounted() {
    await this.loadServerGroups()
    window.addEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal)
    window.addEventListener('file-operation-error', this.handleFileOperationError)
    window.addEventListener('file-operation-success', this.handleFileOperationSuccess)
    Events.On('local-command-output', (event) => this.handleLocalCommandOutput(event.data))
    // 连接/会话状态变化通知
    Events.On('connection-lost', (event) => this.handleConnectionLost(event.data))
    Events.On('terminal-session-closed', (event) => this.handleSessionClosed(event.data))
  },
  beforeUnmount() {
    window.removeEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal)
    window.removeEventListener('file-operation-error', this.handleFileOperationError)
    window.removeEventListener('file-operation-success', this.handleFileOperationSuccess)
    Events.Off('local-command-output')
    Events.Off('connection-lost')
    Events.Off('terminal-session-closed')
  },
  methods: {
    async loadServerGroups() {
      try {
        this.groups = await GetServerGroups()
        const connectionStatus = await GetServerConnectionStatus()
        this.groups.forEach((group) => {
          group.servers.forEach((server) => {
            server.connected = connectionStatus[server.id] || false
          })
        })
        this.closedSessions.clear()
      } catch (error) {
        console.error('加载服务器分组失败:', error)
      }
    },

    onGroupSelect(info) {
      const selectedKeys = info.selectedKeys
      if (selectedKeys.length > 0) {
        const groupId = selectedKeys[0]
        this.selectedGroupKeys = [groupId]
        this.currentGroupId = groupId
        const group = this.groups.find((g) => g.id === groupId)
        if (group) {
          this.currentGroupName = group.name
          this.currentServers = [...group.servers]
        }
      }
    },

    showAddGroupModal() {
      this.editingGroup = null
      this.groupForm.name = ''
      this.groupModalVisible = true
    },
    editGroup(group) {
      this.editingGroup = group
      this.groupForm.name = group.name
      this.groupModalVisible = true
    },
    async handleGroupModalOk() {
      if (!this.groupForm.name.trim()) {
        this.$message.warning('请输入分组名称')
        return
      }
      try {
        const groupData = {
          id: this.editingGroup ? this.editingGroup.id : 'group_' + Date.now(),
          name: this.groupForm.name,
          servers: this.editingGroup ? this.editingGroup.servers : []
        }
        if (this.editingGroup) {
          await UpdateServerGroup(groupData)
        } else {
          await AddServerGroup(groupData)
        }
        this.groupModalVisible = false
        await this.loadServerGroups()
        this.$message.success(`${this.editingGroup ? '更新' : '添加'}分组成功`)
      } catch (error) {
        console.error(`${this.editingGroup ? '更新' : '添加'}分组失败:`, error)
        this.$message.error(`${this.editingGroup ? '更新' : '添加'}分组失败: ${error.message}`)
      }
    },
    async deleteGroup(groupId) {
      try {
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除',
            content: '确定要删除这个分组吗？分组内的服务器也将被删除。',
            okText: '确认', cancelText: '取消',
            onOk: () => resolve(), onCancel: () => reject('cancel')
          })
        })
        await DeleteServerGroup(groupId)
        await this.loadServerGroups()
        this.$message.success('删除分组成功')
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除分组失败:', error)
          this.$message.error(`删除分组失败: ${error.message}`)
        }
      }
    },
    showAddServerModal() {
      this.editingServer = null
      this.serverForm = { name: '', host: '', port: 22, username: '', password: '', keyFile: '', note: '' }
      this.authMethod = 'password'
      this.serverModalVisible = true
    },
    editServer(server) {
      this.editingServer = server
      this.serverForm = { ...server }
      this.authMethod = server.keyFile ? 'key' : 'password'
      this.serverModalVisible = true
    },
    async handleServerModalOk() {
      const form = this.serverForm
      if (!form.name?.trim() || !form.host?.trim() || !form.username?.trim() ||
        (this.authMethod === 'password' && !form.password?.trim()) ||
        (this.authMethod === 'key' && !form.keyFile?.trim())) {
        this.$message.warning('请填写所有必填字段')
        return
      }
      try {
        const serverData = {
          id: this.editingServer ? this.editingServer.id : 'server_' + Date.now(),
          name: form.name, host: form.host, port: form.port, username: form.username,
          password: this.authMethod === 'password' ? form.password : '',
          keyFile: this.authMethod === 'key' ? form.keyFile : '',
          note: form.note || '', groupId: this.currentGroupId
        }
        if (this.editingServer) {
          await UpdateServer(this.currentGroupId, serverData)
        } else {
          await AddServer(this.currentGroupId, serverData)
        }
        this.serverModalVisible = false
        await this.loadServerGroups()
        const group = this.groups.find((g) => g.id === this.currentGroupId)
        if (group) this.currentServers = [...group.servers]
        this.$message.success(`${this.editingServer ? '更新' : '添加'}服务器成功`)
      } catch (error) {
        console.error(`${this.editingServer ? '更新' : '添加'}服务器失败:`, error)
        this.$message.error(`${this.editingServer ? '更新' : '添加'}服务器失败: ${error.message}`)
      }
    },
    async deleteServer(server) {
      try {
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除', content: '确定要删除这个服务器吗？',
            okText: '确认', cancelText: '取消',
            onOk: () => resolve(), onCancel: () => reject('cancel')
          })
        })
        await DeleteServer(server.groupId, server.id)
        await this.loadServerGroups()
        const group = this.groups.find((g) => g.id === this.currentGroupId)
        if (group) this.currentServers = [...group.servers]
        this.$message.success('删除服务器成功')
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除服务器失败:', error)
          this.$message.error(`删除服务器失败: ${error.message}`)
        }
      }
    },
    async connectServer(server) {
      try {
        server.loading = true
        if (server.connected) {
          const result = await DisconnectFromServer(server.id)
          server.connected = false
          if (result && !result.includes('EOF') && !result.includes('断开')) {
            this.$message.success(result)
          }
          this.closedSessions.delete(server.id)
        } else {
          const result = await ConnectToServer(server.id)
          server.connected = true
          this.$message.success(result)
        }
        server.loading = false
        const connectionStatus = await GetServerConnectionStatus()
        server.connected = connectionStatus[server.id] || false
      } catch (error) {
        server.loading = false
        console.error('连接/断开服务器失败:', error)
        if (server.connected) {
          if (error.message && !error.message.includes('EOF') && !error.message.includes('断开')) {
            this.$message.error(`断开服务器连接失败: ${error.message}`)
          }
        } else {
          this.$message.error(`连接服务器失败: ${error.message}`)
        }
      }
    },

    // 打开一个新的终端会话（支持同一服务器多个会话）
    async openTerminal(server) {
      if (!server.connected) {
        try {
          const result = await ConnectToServer(server.id)
          server.connected = true
          this.$message.success(result)
        } catch (error) {
          console.error('连接服务器失败:', error)
          this.$message.error(`连接服务器失败: ${error.message}`)
          return
        }
      }
      this.closedSessions.delete(server.id)
      try {
        const sessionId = await CreateTerminalSessionWithSize(server.id, 80, 24)
        const cnt = this.terminalTabs.filter((t) => t.serverId === server.id && t.type === 'terminal').length + 1
        const tabId = `sess_${sessionId}`
        const newTab = {
          id: tabId, serverId: server.id, sessionId, server,
          title: cnt > 1 ? `${server.name} #${cnt}` : server.name, type: 'terminal'
        }
        this.terminalTabs.push(newTab)
        this.$nextTick(() => { this.activeKey = newTab.id })
      } catch (error) {
        console.error('创建终端会话失败:', error)
        this.$message.error(`创建终端会话失败: ${error.message}`)
      }
    },

    // 复制当前激活的终端会话（快速克隆）
    cloneActiveSession() {
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (!tab) {
        this.$message.info('请先打开一个终端会话再复制')
        return
      }
      this.openTerminal(tab.server)
    },

    async closeTerminalTab(targetKey, action) {
      if (action !== 'remove') return
      const tabIndex = this.terminalTabs.findIndex((tab) => tab.id === targetKey)
      if (tabIndex === -1) return
      const tab = this.terminalTabs[tabIndex]

      if (tab.type === 'terminal' && !this.closedSessions.has(tab.sessionId)) {
        this.closedSessions.add(tab.sessionId)
        try {
          const result = await CloseTerminalSession(tab.sessionId)
          console.log(`终端会话 ${tab.sessionId} 已关闭: ${result}`)
        } catch (error) {
          console.error('关闭终端会话失败:', error)
        }
      }

      this.$nextTick(() => {
        if (!this.$el) return
        this.terminalTabs.splice(tabIndex, 1)
        this.activeKey = this.terminalTabs.length > 0 ? this.terminalTabs[0].id : 'home'
      })
    },

    onTabChange(activeKey) {
      this.activeKey = activeKey
    },

    onTabClick() {
      // 点击标签后确保激活的终端获得焦点（修复光标锁在标题的问题）
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (tab) {
        this.$nextTick(() => {
          const el = this.$el.querySelector('.terminal-element textarea, .terminal-element .xterm-helper-textarea')
          if (el) el.focus()
        })
      }
    },

    manageFiles(server) {
      const existingTab = this.terminalTabs.find((tab) => tab.serverId === server.id && tab.type === 'file')
      if (existingTab) {
        this.activeKey = existingTab.id
        return
      }
      if (!server.connected) {
        this.$message.warning('请先连接服务器')
        return
      }
      const tabId = `file_${server.id}`
      const newTab = {
        id: tabId, serverId: server.id, server,
        title: `${server.name} - 文件管理`, type: 'file'
      }
      this.terminalTabs.push(newTab)
      this.$nextTick(() => { this.activeKey = tabId })
    },

    async handleExecuteScriptInTerminal(event) {
      this.localCommandOutputs = []
      const script = event.detail.script
      if (!script.serverIds || script.serverIds.length === 0) {
        this.$message.error('脚本没有关联的服务器')
        return
      }
      const serverId = script.serverIds[0]
      let server = null
      for (const group of this.groups) {
        const found = group.servers.find((s) => s.id === serverId)
        if (found) { server = found; break }
      }
      if (!server) {
        this.$message.error('找不到指定的服务器')
        return
      }
      if (!server.connected) {
        try {
          const result = await ConnectToServer(server.id)
          server.connected = true
          this.$message.success(result)
        } catch (error) {
          console.error('连接服务器失败:', error)
          this.$message.error(`连接服务器失败: ${error.message}`)
          return
        }
      }

      let tab = this.terminalTabs.find((t) => t.serverId === server.id && t.type === 'terminal')
      if (!tab) {
        const sessionId = await CreateTerminalSessionWithSize(server.id, 80, 24)
        const cnt = 1
        tab = {
          id: `sess_${sessionId}`, serverId: server.id, sessionId, server,
          title: cnt > 1 ? `${server.name} #${cnt}` : server.name, type: 'terminal'
        }
        this.terminalTabs.push(tab)
      }
      this.activeKey = tab.id
      this.pendingScript = { script, sessionId: tab.sessionId }

      // 若终端已挂载，延迟后直接发送；否则等待 terminal-ready
      this.$nextTick(() => {
        const el = this.$el.querySelector('.terminal-element .xterm-helper-textarea')
        if (el) {
          setTimeout(() => {
            if (this.pendingScript && this.pendingScript.sessionId === tab.sessionId) {
              this.sendScriptToTerminal(this.pendingScript.script, tab.sessionId)
              this.pendingScript = null
            }
          }, 1000)
        }
      })
    },

    checkPendingScript(sessionId) {
      if (this.pendingScript && this.pendingScript.sessionId === sessionId) {
        setTimeout(() => {
          this.sendScriptToTerminal(this.pendingScript.script, sessionId)
          this.pendingScript = null
        }, 1000)
      }
    },

    async sendScriptToTerminal(script, sessionId) {
      try {
        this.$message.loading('正在处理脚本命令...', 0)
        await SendScriptToTerminal(script.id, sessionId)
        this.$message.destroy()
        this.$message.success('脚本命令已发送到终端')
      } catch (error) {
        this.$message.destroy()
        console.error('发送脚本命令失败:', error)
        this.$message.error(`发送脚本命令失败: ${error.message}`)
      }
    },

    // 连接断开通知
    handleConnectionLost(data) {
      if (!data) return
      const serverID = data.serverID
      this.$notification.error({
        message: '服务器连接已断开',
        description: `${data.reason || '连接丢失'}`,
        duration: 4
      })
      // 关闭该服务器下所有终端/文件标签
      const toRemove = this.terminalTabs.filter((t) => t.serverId === serverID)
      toRemove.forEach((tab) => {
        if (tab.type === 'terminal') {
          try { CloseTerminalSession(tab.sessionId) } catch (e) { /* ignore */ }
        }
      })
      this.terminalTabs = this.terminalTabs.filter((t) => t.serverId !== serverID)
      // 更新连接状态
      for (const g of this.groups) {
        g.servers.forEach((s) => { if (s.id === serverID) s.connected = false })
      }
      if (this.activeKey !== 'home' && !this.terminalTabs.some((t) => t.id === this.activeKey)) {
        this.activeKey = 'home'
      }
    },

    // 单个终端会话意外关闭
    handleSessionClosed(data) {
      if (!data) return
      const sessionID = data.sessionID
      const tab = this.terminalTabs.find((t) => t.sessionId === sessionID)
      if (!tab) return
      this.$notification.warning({
        message: '终端会话已结束',
        description: `${tab.server?.name || ''} 的会话已断开：${data.reason || '连接关闭'}`,
        duration: 4
      })
      this.terminalTabs = this.terminalTabs.filter((t) => t.sessionId !== sessionID)
      if (this.activeKey === tab.id) {
        this.activeKey = this.terminalTabs.length > 0 ? this.terminalTabs[0].id : 'home'
      }
    },

    handleFileOperationSuccess(event) {
      const { type, localPath, remotePath } = event.detail
      let message = ''
      if (type === 'upload') message = `文件上传成功: ${localPath} -> ${remotePath}`
      else if (type === 'download') message = `文件下载成功: ${remotePath} -> ${localPath}`
      this.$message.success(message)
    },
    handleLocalCommandOutput(data) {
      const { command, output } = data
      this.localCommandOutputs.push({ command, output })
      this.localCommandOutputVisible = true
    },
    handleFileOperationError(event) {
      const { type, localPath, remotePath, error } = event.detail
      let message = ''
      if (type === 'upload') message = `文件上传失败: ${localPath} -> ${remotePath}, 错误: ${error}`
      else if (type === 'download') message = `文件下载失败: ${remotePath} -> ${localPath}, 错误: ${error}`
      this.$message.error(message)
    }
  }
}
</script>

<style scoped>
.main-tabs-container { height: 100vh; }
.layout { height: 100vh; }
.group-section { padding: 16px; }
.group-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.group-header h3 { margin: 0; }
.group-actions { float: right; }
.group-actions .anticon { margin-left: 8px; cursor: pointer; }
.content { padding: 16px; }

.local-command-output { padding: 8px; }
.output-container { max-height: 60vh; overflow-y: auto; }
.command-item { margin-bottom: 20px; padding-bottom: 16px; border-bottom: 1px dashed #d9d9d9; }
.command-item:last-child { margin-bottom: 0; padding-bottom: 0; border-bottom: none; }
.command-header { margin-bottom: 8px; }
.command-number {
  display: inline-block; width: 24px; height: 24px; line-height: 24px; text-align: center;
  background-color: #1890ff; color: white; border-radius: 50%; font-size: 12px; margin-right: 8px;
}
.command-header code { margin-left: 8px; color: #1890ff; font-family: 'Monaco', 'Menlo', monospace; }
.output-header { margin-bottom: 8px; color: #666; }
.output-content {
  margin: 0; padding: 12px; background-color: #1f1f1f; color: #e8e8e8; border-radius: 4px;
  font-family: 'Monaco', 'Menlo', monospace; font-size: 13px; line-height: 1.5;
  white-space: pre-wrap; word-break: break-all; max-height: 40vh; overflow-y: auto;
}
</style>
