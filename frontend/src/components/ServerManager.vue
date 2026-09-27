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
            <a-button size="small" :disabled="!activeIsTerminal" @click="splitActiveTerminal">
              <BorderOutlined />分屏
            </a-button>
            <a-badge :count="taskStore.unread" :offset="[-4, 2]">
              <a-button size="small" @click="taskStore.openDrawer()">
                <UnorderedListOutlined />任务中心
              </a-button>
            </a-badge>
            <a-button size="small" @click="showSnippetsModal">
              <AppstoreOutlined />片段库
            </a-button>
            <a-button size="small" @click="showBroadcastModal">
              <SendOutlined />广播
            </a-button>
            <a-button size="small" @click="showTunnelModal">
              <ApiOutlined />隧道
            </a-button>
            <a-button size="small" @click="showSettingsModal">
              <SettingOutlined />设置
            </a-button>
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
                  <a-input
                    v-model:value="quickURI"
                    placeholder="快速连接：ssh://user@host:port 或 user@host"
                    style="width: 360px; margin-left: 12px"
                    @pressEnter="quickConnect"
                  >
                    <template #prefix><LinkOutlined /></template>
                  </a-input>
                  <a-button style="margin-left: 8px" :loading="quickConnecting" @click="quickConnect">
                    连接
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
                          <FolderOpenOutlined />文件
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

        <!-- 终端 / 文件 标签页（支持同服务器多会话、分屏） -->
        <a-tab-pane v-for="tab in terminalTabs" :key="tab.id" :tab="tab.title" closable force-render>
          <div v-if="tab.type === 'terminal'" style="height: 100%; padding: 0; margin: 0">
            <div class="terminal-split">
              <div v-for="pane in tab.panes" :key="pane.id" class="terminal-pane"
                :data-pane-id="pane.id" :class="{ active: pane.active }" @click="setActivePane(tab, pane)">
                <div class="pane-bar">
                  <span class="pane-title">{{ pane.title }}</span>
                  <a-button v-if="tab.panes.length > 1" size="small" type="text"
                    @click.stop="closePane(tab, pane)">✕</a-button>
                </div>
                <div class="pane-body">
                  <Terminal :server="pane.server" :server-id="pane.serverId" :session-id="pane.sessionId"
                    :active="pane.active" @terminal-ready="checkPendingScript" />
                </div>
              </div>
            </div>
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
        <a-form-item label="跳板机">
          <a-select v-model:value="serverForm.proxyJumpServerId" allowClear placeholder="可选：通过此服务器代理连接">
            <a-select-option v-for="s in allServers" :key="s.id" :value="s.id">
              {{ s.name }} ({{ s.host }})
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="自动重连">
          <a-switch v-model:checked="autoReconnect" />
          <span class="hint">断线后自动尝试重连（指数退避，最多 8 次）</span>
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="serverForm.note" placeholder="请输入备注信息（可选）" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 片段库模态框 -->
    <a-modal v-model:open="snippetsVisible" title="命令片段库" width="720px" :footer="null">
      <div class="snippet-toolbar">
        <a-button type="primary" size="small" @click="openSnippetEdit()"><PlusOutlined />新建片段</a-button>
        <span class="hint">点击「发送」将内容发送到当前终端</span>
      </div>
      <a-table :dataSource="snippets" :columns="snippetColumns" :pagination="false" rowKey="id" size="small">
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'content'">
            <code class="snippet-content">{{ record.content }}</code>
          </template>
          <template v-else-if="column.dataIndex === 'action'">
            <a-space>
              <a-button size="small" type="primary" @click="sendSnippet(record)">发送</a-button>
              <a-button size="small" @click="openSnippetEdit(record)">编辑</a-button>
              <a-button size="small" danger @click="deleteSnippet(record)">删除</a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-modal>

    <!-- 片段编辑模态框 -->
    <a-modal v-model:open="snippetEditVisible" :title="editingSnippet ? '编辑片段' : '新建片段'" @ok="saveSnippet">
      <a-form layout="vertical">
        <a-form-item label="名称" required>
          <a-input v-model:value="snippetForm.name" placeholder="如：查看磁盘" />
        </a-form-item>
        <a-form-item label="关联服务器（可选）">
          <a-select v-model:value="snippetForm.serverId" allowClear placeholder="通用（不绑定）">
            <a-select-option v-for="s in allServers" :key="s.id" :value="s.id">{{ s.name }}</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="命令内容" required>
          <a-textarea v-model:value="snippetForm.content" :rows="5" placeholder="可多行，每行一条命令" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 广播模态框 -->
    <a-modal v-model:open="broadcastVisible" title="命令广播（发送到多个终端）" width="640px" @ok="sendBroadcast">
      <a-input v-model:value="broadcastCmd" placeholder="输入要广播的命令，将发送到所选终端" />
      <div class="broadcast-list">
        <a-checkbox v-model:checked="broadcastAll" @change="onBroadcastAll">全选</a-checkbox>
        <div v-for="item in broadcastTargets" :key="item.key" class="broadcast-item">
          <a-checkbox v-model:checked="item.checked">{{ item.label }}</a-checkbox>
        </div>
        <div v-if="!broadcastTargets.length" class="hint">当前没有打开的终端会话</div>
      </div>
    </a-modal>

    <!-- 隧道（端口转发）模态框 -->
    <a-modal v-model:open="tunnelVisible" title="SSH 隧道 / 端口转发" width="720px" :footer="null">
      <a-form layout="inline" style="margin-bottom: 12px">
        <a-form-item label="服务器" required>
          <a-select v-model:value="tunnelServerId" style="width: 240px" @change="refreshTunnels">
            <a-select-option v-for="s in connectedServers" :key="s.id" :value="s.id">{{ s.name }}</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="类型">
          <a-radio-group v-model:value="tunnelType">
            <a-radio value="local">本地转发</a-radio>
            <a-radio value="remote">远程转发</a-radio>
          </a-radio-group>
        </a-form-item>
      </a-form>
      <a-form layout="inline" v-if="tunnelServerId">
        <template v-if="tunnelType === 'local'">
          <a-form-item label="本地监听" required>
            <a-input v-model:value="tLocalAddr" placeholder="127.0.0.1:8080" style="width: 160px" />
          </a-form-item>
          <a-form-item label="远端目标" required>
            <a-input v-model:value="tRemoteHost" placeholder="127.0.0.1" style="width: 130px" />
          </a-form-item>
          <a-form-item label="远端端口" required>
            <a-input v-model:value="tRemotePort" placeholder="80" style="width: 90px" />
          </a-form-item>
        </template>
        <template v-else>
          <a-form-item label="远端监听" required>
            <a-input v-model:value="tRemoteAddr" placeholder="127.0.0.1:8080" style="width: 160px" />
          </a-form-item>
          <a-form-item label="本地目标" required>
            <a-input v-model:value="tLocalAddr" placeholder="127.0.0.1:80" style="width: 160px" />
          </a-form-item>
        </template>
        <a-form-item>
          <a-button type="primary" @click="addTunnel">添加转发</a-button>
        </a-form-item>
      </a-form>
      <a-table :dataSource="tunnels" :columns="tunnelColumns" :pagination="false" rowKey="id" size="small" style="margin-top: 12px">
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'type'">
            <a-tag :color="record.type === 'local' ? 'blue' : 'purple'">
              {{ record.type === 'local' ? '本地转发' : '远程转发' }}
            </a-tag>
          </template>
          <template v-else-if="column.dataIndex === 'action'">
            <a-button size="small" danger @click="removeTunnel(record)">停止</a-button>
          </template>
        </template>
      </a-table>
    </a-modal>

    <!-- 分屏：选择要并排的服务器 -->
    <a-modal v-model:open="splitPickerVisible" title="分屏：选择要并排的服务器" width="560px" :footer="null">
      <a-alert type="info" show-icon style="margin-bottom: 12px"
        message="选中的服务器终端会并排到当前视图；可多次分屏，也可选当前服务器做多 Shell。" />
      <a-table :dataSource="allServers" :columns="splitColumns" :pagination="false" rowKey="id" size="small">
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <a-tag :color="record.connected ? 'green' : 'default'">{{ record.connected ? '已连接' : '未连接' }}</a-tag>
          </template>
          <template v-else-if="column.dataIndex === 'action'">
            <a-button size="small" type="primary" @click="confirmSplit(record)">分屏到此</a-button>
          </template>
        </template>
      </a-table>
    </a-modal>

    <!-- 设置模态框 -->
    <a-modal v-model:open="settingsVisible" title="设置" @ok="saveSettings">
      <a-form layout="vertical">
        <a-form-item label="主题">
          <a-radio-group v-model:value="settings.theme">
            <a-radio value="dark">深色</a-radio>
            <a-radio value="light">浅色</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="字号">
          <a-input-number v-model:value="settings.fontSize" :min="10" :max="28" />
        </a-form-item>
        <a-form-item label="光标样式">
          <a-select v-model:value="settings.cursorStyle" style="width: 160px">
            <a-select-option value="block">方块</a-select-option>
            <a-select-option value="bar">竖线</a-select-option>
            <a-select-option value="underline">下划线</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="背景色">
          <a-input type="color" v-model:value="settings.background" style="width: 80px" />
        </a-form-item>
        <a-form-item label="前景色">
          <a-input type="color" v-model:value="settings.foreground" style="width: 80px" />
        </a-form-item>
        <a-form-item label="选中高亮">
          <a-input type="color" v-model:value="settings.selection" style="width: 80px" />
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
            <div class="output-header"><strong>输出:</strong></div>
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
  ConnectByURI,
  DeleteServer,
  DeleteServerGroup,
  DisconnectFromServer,
  GetServerGroups,
  GetServerConnectionStatus,
  UpdateServer,
  UpdateServerGroup,
  CreateTerminalSessionWithSize,
  SendScriptToTerminal,
  ExecuteCommandWithoutNewline,
  AddCommandHistory,
  GetSnippets,
  AddSnippet,
  UpdateSnippet,
  DeleteSnippet,
  AddPortForward,
  RemovePortForward,
  ListPortForwards,
  EnableAutoReconnect,
  GetAutoReconnect
} from '../../bindings/go-term/controllers/sshcontroller'
import { Events } from '@wailsio/runtime'
import Terminal from './Terminal.vue'
import FileManager from './FileManager.vue'
import BatchScriptManager from './BatchScriptManager.vue'
import OpsConfigManager from './OpsConfigManager.vue'
import TaskCenter from './TaskCenter.vue'
import { taskStore } from '../store/taskStore.js'
import { settingsStore, saveSettings as persistSettings } from '../store/settingsStore.js'
import {
  FolderOutlined,
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  WifiOutlined,
  CodeOutlined,
  CopyOutlined,
  UnorderedListOutlined,
  FolderOpenOutlined,
  AppstoreOutlined,
  SendOutlined,
  ApiOutlined,
  SettingOutlined,
  LinkOutlined,
  BorderOutlined
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
    UnorderedListOutlined,
    FolderOpenOutlined,
    AppstoreOutlined,
    SendOutlined,
    ApiOutlined,
    SettingOutlined,
    LinkOutlined,
    BorderOutlined
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
      autoReconnect: false,
      serverForm: {
        name: '', host: '', port: 22, username: 'root', password: '', keyFile: '', note: '', proxyJumpServerId: undefined
      },

      quickURI: '',
      quickConnecting: false,

      // 片段库
      snippetsVisible: false,
      snippets: [],
      snippetEditVisible: false,
      editingSnippet: null,
      snippetForm: { name: '', content: '', serverId: undefined },

      // 广播
      broadcastVisible: false,
      broadcastCmd: '',
      broadcastAll: false,
      broadcastTargets: [],

      // 隧道
      tunnelVisible: false,
      tunnelServerId: undefined,
      tunnelType: 'local',
      tLocalAddr: '', tRemoteHost: '', tRemotePort: '', tRemoteAddr: '',
      tunnels: [],

      // 分屏选服务器
      splitPickerVisible: false,

      // 设置
      settingsVisible: false,
      settings: { ...settingsStore },

      serverColumns: [
        { title: '服务器名称', dataIndex: 'name', key: 'name' },
        { title: '主机地址', dataIndex: 'host', key: 'host' },
        { title: '端口', dataIndex: 'port', key: 'port' },
        { title: '状态', dataIndex: 'status', key: 'status' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ],
      splitColumns: [
        { title: '服务器名称', dataIndex: 'name', key: 'name' },
        { title: '主机地址', dataIndex: 'host', key: 'host' },
        { title: '状态', dataIndex: 'status', key: 'status' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ],
      snippetColumns: [
        { title: '名称', dataIndex: 'name', key: 'name' },
        { title: '内容', dataIndex: 'content', key: 'content' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ],
      tunnelColumns: [
        { title: '类型', dataIndex: 'type', key: 'type' },
        { title: '本地', dataIndex: 'localAddr', key: 'localAddr' },
        { title: '远端主机', dataIndex: 'remoteHost', key: 'remoteHost' },
        { title: '远端端口', dataIndex: 'remotePort', key: 'remotePort' },
        { title: '状态', dataIndex: 'status', key: 'status' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    }
  },
  computed: {
    taskStore() {
      return taskStore
    },
    activeIsTerminal() {
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey)
      return !!(tab && tab.type === 'terminal')
    },
    allServers() {
      const list = []
      this.groups.forEach((g) => g.servers.forEach((s) => list.push(s)))
      return list
    },
    connectedServers() {
      return this.allServers.filter((s) => s.connected)
    }
  },
  async mounted() {
    await this.loadServerGroups()
    window.addEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal)
    window.addEventListener('file-operation-error', this.handleFileOperationError)
    window.addEventListener('file-operation-success', this.handleFileOperationSuccess)
    Events.On('local-command-output', (event) => this.handleLocalCommandOutput(event.data))
    Events.On('connection-lost', (event) => this.handleConnectionLost(event.data))
    Events.On('terminal-session-closed', (event) => this.handleSessionClosed(event.data))
    Events.On('reconnecting', (event) => this.handleReconnecting(event.data))
    Events.On('reconnected', (event) => this.handleReconnected(event.data))
  },
  beforeUnmount() {
    window.removeEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal)
    window.removeEventListener('file-operation-error', this.handleFileOperationError)
    window.removeEventListener('file-operation-success', this.handleFileOperationSuccess)
    Events.Off('local-command-output')
    Events.Off('connection-lost')
    Events.Off('terminal-session-closed')
    Events.Off('reconnecting')
    Events.Off('reconnected')
  },
  methods: {
    /* ========== 基础 ========== */
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
        this.selectedGroupKeys = [selectedKeys[0]]
        this.currentGroupId = selectedKeys[0]
        const group = this.groups.find((g) => g.id === selectedKeys[0])
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
      this.serverForm = { name: '', host: '', port: 22, username: '', password: '', keyFile: '', note: '', proxyJumpServerId: undefined }
      this.authMethod = 'password'
      this.autoReconnect = false
      this.serverModalVisible = true
    },
    editServer(server) {
      this.editingServer = server
      this.serverForm = { ...server }
      this.authMethod = server.keyFile ? 'key' : 'password'
      this.autoReconnect = false
      this.serverModalVisible = true
      // 读取自动重连开关
      GetAutoReconnect(server.id).then((v) => { this.autoReconnect = !!v }).catch(() => {})
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
          note: form.note || '', groupId: this.currentGroupId,
          proxyJumpServerId: form.proxyJumpServerId || ''
        }
        if (this.editingServer) {
          await UpdateServer(this.currentGroupId, serverData)
        } else {
          await AddServer(this.currentGroupId, serverData)
        }
        // 自动重连开关（仅编辑时或新建后均设置）
        await EnableAutoReconnect(serverData.id, this.autoReconnect).catch(() => {})
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

    /* ========== 终端 / 分屏 ========== */
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
        const pane = {
          id: 'pane_' + sessionId, sessionId, serverId: server.id, server, title: server.name, active: true
        }
        const cnt = this.terminalTabs.filter((t) => t.type === 'terminal' && t.serverId === server.id).length + 1
        const tabId = `sess_${sessionId}`
        const newTab = {
          id: tabId, type: 'terminal', serverId: server.id, server,
          title: cnt > 1 ? `${server.name} #${cnt}` : server.name,
          panes: [pane], activePane: pane.id
        }
        this.terminalTabs.push(newTab)
        this.$nextTick(() => { this.activeKey = newTab.id })
      } catch (error) {
        console.error('创建终端会话失败:', error)
        this.$message.error(`创建终端会话失败: ${error.message}`)
      }
    },

    cloneActiveSession() {
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (!tab) {
        this.$message.info('请先打开一个终端会话再复制')
        return
      }
      this.openTerminal(tab.server)
    },

    async splitActiveTerminal() {
      if (!this.activeIsTerminal) {
        this.$message.info('请先打开一个终端再分屏')
        return
      }
      // 打开选服务器弹窗，把不同服务器并排到当前视图
      this.splitPickerVisible = true
    },

    async confirmSplit(server) {
      this.splitPickerVisible = false
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (!tab) {
        this.$message.info('请先打开一个终端再分屏')
        return
      }
      try {
        if (!server.connected) {
          this.$message.info(`正在连接 ${server.name} ...`)
          const res = await ConnectToServer(server.id)
          server.connected = true
          this.$message.success(res || `已连接 ${server.name}`)
        }
        const sessionId = await CreateTerminalSessionWithSize(server.id, 80, 24)
        tab.panes.forEach((p) => (p.active = false))
        const pane = {
          id: 'pane_' + sessionId, sessionId, serverId: server.id, server, title: server.name, active: true
        }
        tab.panes.push(pane)
        tab.activePane = pane.id
        this.activeKey = tab.id
        this.$nextTick(() => {
          const el = this.$el.querySelector(`.terminal-pane[data-pane-id="${pane.id}"] .xterm-helper-textarea`)
          if (el) el.focus()
        })
      } catch (error) {
        console.error('分屏失败:', error)
        this.$message.error(`分屏失败: ${error.message}`)
      }
    },

    setActivePane(tab, pane) {
      if (pane.active) {
        // 已激活，直接把光标送进该分屏
        this.$nextTick(() => {
          const el = this.$el.querySelector(`.terminal-pane[data-pane-id="${pane.id}"] .xterm-helper-textarea`)
          if (el) el.focus()
        })
        return
      }
      tab.panes.forEach((p) => (p.active = false))
      pane.active = true
      tab.activePane = pane.id
      this.$nextTick(() => {
        const el = this.$el.querySelector(`.terminal-pane[data-pane-id="${pane.id}"] .xterm-helper-textarea`)
        if (el) el.focus()
      })
    },

    async closePane(tab, pane) {
      try {
        await CloseTerminalSession(pane.sessionId)
      } catch (e) { /* ignore */ }
      if (tab.panes.length <= 1) {
        // 关闭整个标签
        const idx = this.terminalTabs.findIndex((t) => t.id === tab.id)
        if (idx >= 0) this.terminalTabs.splice(idx, 1)
        this.activeKey = this.terminalTabs.length > 0 ? this.terminalTabs[0].id : 'home'
      } else {
        const idx = tab.panes.findIndex((p) => p.id === pane.id)
        if (idx >= 0) tab.panes.splice(idx, 1)
        if (pane.active && tab.panes.length) {
          tab.panes[tab.panes.length - 1].active = true
          tab.activePane = tab.panes[tab.panes.length - 1].id
        }
      }
    },

    async closeTerminalTab(targetKey, action) {
      if (action !== 'remove') return
      const tabIndex = this.terminalTabs.findIndex((tab) => tab.id === targetKey)
      if (tabIndex === -1) return
      const tab = this.terminalTabs[tabIndex]

      if (tab.type === 'terminal' && !this.closedSessions.has(tab.id)) {
        this.closedSessions.add(tab.id)
        for (const pane of tab.panes) {
          try {
            if (!this.closedSessions.has(pane.sessionId)) {
              this.closedSessions.add(pane.sessionId)
              await CloseTerminalSession(pane.sessionId)
            }
          } catch (error) {
            console.error('关闭终端会话失败:', error)
          }
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
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (tab) {
        const activePane = tab.panes.find((p) => p.active) || tab.panes[0]
        if (activePane) {
          this.$nextTick(() => {
            const el = this.$el.querySelector(`.terminal-pane[data-pane-id="${activePane.id}"] .xterm-helper-textarea`)
            if (el) el.focus()
          })
        }
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

    /* ========== 快速连接 ========== */
    async quickConnect() {
      const uri = (this.quickURI || '').trim()
      if (!uri) {
        this.$message.warning('请输入连接地址')
        return
      }
      this.quickConnecting = true
      try {
        const serverID = await ConnectByURI(uri)
        // 找到新建服务器并打开终端
        let server = null
        for (const g of this.groups) {
          const f = g.servers.find((s) => s.id === serverID)
          if (f) { server = f; break }
        }
        await this.loadServerGroups()
        if (server) {
          const refreshed = this.allServers.find((s) => s.id === serverID)
          if (refreshed) this.openTerminal(refreshed)
        }
        this.quickURI = ''
        this.$message.success('快速连接成功')
      } catch (error) {
        console.error('快速连接失败:', error)
        this.$message.error(`快速连接失败: ${error.message}`)
      } finally {
        this.quickConnecting = false
      }
    },

    /* ========== 片段库 ========== */
    async showSnippetsModal() {
      try {
        this.snippets = await GetSnippets()
      } catch (e) {
        this.snippets = []
      }
      this.snippetsVisible = true
    },
    openSnippetEdit(record) {
      if (record) {
        this.editingSnippet = record
        this.snippetForm = { name: record.name, content: record.content, serverId: record.serverId }
      } else {
        this.editingSnippet = null
        this.snippetForm = { name: '', content: '', serverId: undefined }
      }
      this.snippetEditVisible = true
    },
    async saveSnippet() {
      if (!this.snippetForm.name?.trim() || !this.snippetForm.content?.trim()) {
        this.$message.warning('请填写名称与命令内容')
        return
      }
      try {
        if (this.editingSnippet) {
          await UpdateSnippet({ ...this.editingSnippet, ...this.snippetForm })
        } else {
          await AddSnippet({ ...this.snippetForm })
        }
        this.snippetEditVisible = false
        this.snippets = await GetSnippets()
        this.$message.success('片段已保存')
      } catch (error) {
        this.$message.error(`保存失败: ${error.message}`)
      }
    },
    async deleteSnippet(record) {
      try {
        await DeleteSnippet(record.id)
        this.snippets = await GetSnippets()
        this.$message.success('已删除')
      } catch (error) {
        this.$message.error(`删除失败: ${error.message}`)
      }
    },
    sendSnippet(record) {
      const pane = this.findActivePane()
      if (!pane) {
        this.$message.warning('请先打开并选中一个终端')
        return
      }
      const cmd = record.content
      ExecuteCommandWithoutNewline(pane.sessionId, cmd + '\r').catch((e) => console.error(e))
      AddCommandHistory(pane.serverId, cmd).catch(() => {})
      this.snippetsVisible = false
      this.$message.success('已发送到当前终端')
    },

    /* ========== 广播 ========== */
    showBroadcastModal() {
      this.collectBroadcastTargets()
      this.broadcastCmd = ''
      this.broadcastAll = false
      this.broadcastVisible = true
    },
    collectBroadcastTargets() {
      const targets = []
      this.terminalTabs.forEach((tab) => {
        if (tab.type === 'terminal') {
          tab.panes.forEach((pane) => {
            targets.push({
              key: pane.sessionId,
              label: `${tab.title} · ${pane.server ? pane.server.name : pane.serverId}`,
              sessionId: pane.sessionId,
              serverId: pane.serverId,
              checked: false
            })
          })
        }
      })
      this.broadcastTargets = targets
    },
    onBroadcastAll(e) {
      const all = e.target.checked
      this.broadcastTargets.forEach((t) => (t.checked = all))
    },
    async sendBroadcast() {
      const cmd = (this.broadcastCmd || '').trim()
      if (!cmd) {
        this.$message.warning('请输入要广播的命令')
        return
      }
      const selected = this.broadcastTargets.filter((t) => t.checked)
      if (!selected.length) {
        this.$message.warning('请至少选择一个终端')
        return
      }
      for (const t of selected) {
        ExecuteCommandWithoutNewline(t.sessionId, cmd + '\r').catch((e) => console.error(e))
        AddCommandHistory(t.serverId, cmd).catch(() => {})
      }
      this.broadcastVisible = false
      this.$message.success(`已向 ${selected.length} 个终端广播命令`)
    },

    /* ========== 隧道 / 端口转发 ========== */
    showTunnelModal() {
      this.tunnelServerId = this.connectedServers.length ? this.connectedServers[0].id : undefined
      this.tunnels = []
      this.refreshTunnels()
      this.tunnelVisible = true
    },
    async refreshTunnels() {
      if (!this.tunnelServerId) {
        this.tunnels = []
        return
      }
      try {
        this.tunnels = await ListPortForwards(this.tunnelServerId)
      } catch (e) {
        this.tunnels = []
      }
    },
    async addTunnel() {
      if (!this.tunnelServerId) {
        this.$message.warning('请选择服务器')
        return
      }
      let fwd
      if (this.tunnelType === 'local') {
        if (!this.tLocalAddr || !this.tRemoteHost || !this.tRemotePort) {
          this.$message.warning('请填写本地监听与远端目标')
          return
        }
        fwd = {
          type: 'local', localAddr: this.tLocalAddr,
          remoteHost: this.tRemoteHost, remotePort: this.tRemotePort
        }
      } else {
        if (!this.tRemoteAddr || !this.tLocalAddr) {
          this.$message.warning('请填写远端监听与本地目标')
          return
        }
        fwd = {
          type: 'remote', remoteAddr: this.tRemoteAddr, localAddr: this.tLocalAddr
        }
      }
      try {
        await AddPortForward(this.tunnelServerId, fwd)
        this.$message.success('转发已启动')
        this.tLocalAddr = ''; this.tRemoteHost = ''; this.tRemotePort = ''; this.tRemoteAddr = ''
        await this.refreshTunnels()
      } catch (error) {
        this.$message.error(`添加失败: ${error.message}`)
      }
    },
    async removeTunnel(record) {
      try {
        await RemovePortForward(record.id)
        this.$message.success('已停止')
        await this.refreshTunnels()
      } catch (error) {
        this.$message.error(`停止失败: ${error.message}`)
      }
    },

    /* ========== 设置 ========== */
    showSettingsModal() {
      this.settings = { ...settingsStore }
      this.settingsVisible = true
    },
    saveSettings() {
      Object.assign(settingsStore, this.settings)
      persistSettings()
      window.dispatchEvent(new Event('apply-theme'))
      this.settingsVisible = false
      this.$message.success('设置已保存')
    },

    /* ========== 脚本执行到终端（保持原逻辑） ========== */
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
        const pane = { id: 'pane_' + sessionId, sessionId, serverId: server.id, server, title: server.name, active: true }
        tab = {
          id: `sess_${sessionId}`, type: 'terminal', serverId: server.id, server,
          title: server.name, panes: [pane], activePane: pane.id
        }
        this.terminalTabs.push(tab)
      }
      this.activeKey = tab.id
      this.pendingScript = { script, sessionId: tab.panes[0].sessionId }

      this.$nextTick(() => {
        const el = this.$el.querySelector('.terminal-element .xterm-helper-textarea')
        if (el) {
          setTimeout(() => {
            if (this.pendingScript && this.pendingScript.sessionId === tab.panes[0].sessionId) {
              this.sendScriptToTerminal(this.pendingScript.script, tab.panes[0].sessionId)
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

    /* ========== 连接状态事件 ========== */
    handleConnectionLost(data) {
      if (!data) return
      const serverID = data.serverID
      this.$notification.error({
        message: '服务器连接已断开',
        description: `${data.reason || '连接丢失'}`,
        duration: 4
      })
      // 关闭该服务器下所有分屏会话（按 pane 维度，支持同一视图并排多台服务器）
      const removedTabIds = new Set()
      this.terminalTabs.forEach((tab) => {
        if (tab.type !== 'terminal') return
        tab.panes.forEach((p) => {
          if (p.serverId === serverID) {
            try { CloseTerminalSession(p.sessionId) } catch (e) { /* ignore */ }
          }
        })
        tab.panes = tab.panes.filter((p) => p.serverId !== serverID)
        if (tab.panes.length === 0) removedTabIds.add(tab.id)
      })
      if (removedTabIds.size) {
        this.terminalTabs = this.terminalTabs.filter((t) => !removedTabIds.has(t.id))
      }
      for (const g of this.groups) {
        g.servers.forEach((s) => { if (s.id === serverID) s.connected = false })
      }
      if (this.activeKey !== 'home' && !this.terminalTabs.some((t) => t.id === this.activeKey)) {
        this.activeKey = 'home'
      }
    },

    handleReconnecting(data) {
      if (!data) return
      this.$notification.info({
        message: '连接恢复中',
        description: `${data.reason || '正在尝试自动重连…'}`,
        duration: 2
      })
    },

    async handleReconnected(data) {
      if (!data) return
      const serverID = data.serverID
      // 更新连接状态
      for (const g of this.groups) {
        g.servers.forEach((s) => { if (s.id === serverID) s.connected = true })
      }
      // 复活该服务器下所有打开的终端会话（按 pane 维度重建 pty）
      for (const tab of this.terminalTabs) {
        if (tab.type !== 'terminal') continue
        for (const pane of tab.panes) {
          if (pane.serverId !== serverID) continue
          try { await CloseTerminalSession(pane.sessionId) } catch (e) { /* ignore */ }
          try {
            const sid = await CreateTerminalSessionWithSize(serverID, 80, 24)
            pane.sessionId = sid
            pane.id = 'pane_' + sid
          } catch (e) {
            console.error('重连后重建会话失败:', e)
          }
        }
      }
      this.$notification.success({
        message: '连接已恢复',
        description: '终端会话已自动重建',
        duration: 3
      })
    },

    handleSessionClosed(data) {
      if (!data) return
      const sessionID = data.sessionID
      const tab = this.terminalTabs.find((t) => t.type === 'terminal' && t.panes.some((p) => p.sessionId === sessionID))
      if (!tab) return
      this.$notification.warning({
        message: '终端会话已结束',
        description: `${tab.server?.name || ''} 的会话已断开：${data.reason || '连接关闭'}`,
        duration: 4
      })
      // 移除该分屏
      const idx = tab.panes.findIndex((p) => p.sessionId === sessionID)
      if (idx >= 0) tab.panes.splice(idx, 1)
      if (tab.panes.length === 0) {
        this.terminalTabs = this.terminalTabs.filter((t) => t.id !== tab.id)
      } else if (tab.panes[idx]) {
        tab.panes[idx].active = true
        tab.activePane = tab.panes[idx].id
      }
      if (this.activeKey === tab.id && tab.panes.length === 0) {
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
    },

    /* ========== 工具 ========== */
    findActivePane() {
      const tab = this.terminalTabs.find((t) => t.id === this.activeKey && t.type === 'terminal')
      if (!tab) {
        // 退而求其次：取第一个终端标签的激活分屏
        const any = this.terminalTabs.find((t) => t.type === 'terminal')
        if (!any) return null
        return any.panes.find((p) => p.active) || any.panes[0] || null
      }
      return tab.panes.find((p) => p.active) || tab.panes[0] || null
    }
  }
}
</script>

<style scoped>
.main-tabs-container { height: 100vh; display: flex; flex-direction: column; overflow: hidden; }
/* 让 a-tabs 内容区撑满剩余高度，终端才能拿到正确高度 */
.main-tabs-container :deep(.ant-tabs) { height: 100%; display: flex; flex-direction: column; min-height: 0; }
.main-tabs-container :deep(.ant-tabs-content-holder) { flex: 1 1 auto; min-height: 0; overflow: hidden; }
.main-tabs-container :deep(.ant-tabs-content) { height: 100%; }
.main-tabs-container :deep(.ant-tabs-tabpane) { height: 100%; }
.layout { height: 100%; }
.group-section { padding: 16px; }
.group-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.group-header h3 { margin: 0; }
.group-actions { float: right; }
.group-actions .anticon { margin-left: 8px; cursor: pointer; }
.content { padding: 16px; }

.server-header { display: flex; align-items: center; margin-bottom: 12px; flex-wrap: wrap; }
.hint { color: #999; font-size: 12px; margin-left: 8px; }

.terminal-split { display: flex; height: 100%; width: 100%; }
.terminal-pane {
  flex: 1; min-width: 0; display: flex; flex-direction: column;
  border: 1px solid transparent;
}
.terminal-pane.active { border-color: #1890ff; }
.terminal-pane + .terminal-pane { border-left: 1px solid #333; }
.pane-bar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 2px 8px; background: #252526; color: #bbb; font-size: 12px; user-select: none;
}
.pane-title { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pane-body { flex: 1; min-height: 0; }

.snippet-toolbar { display: flex; align-items: center; margin-bottom: 12px; gap: 12px; }
.snippet-content {
  font-family: 'Consolas', monospace; font-size: 12px; color: #e8e8e8;
  background: #1f1f1f; padding: 2px 6px; border-radius: 3px; white-space: pre-wrap;
}
.broadcast-list { margin-top: 12px; max-height: 320px; overflow-y: auto; }
.broadcast-item { padding: 4px 0; }

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
