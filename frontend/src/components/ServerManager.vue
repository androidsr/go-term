<template>
  <div>
    <div class="main-tabs-container">
      <a-tabs v-model:activeKey="activeKey" size="small" :hideAdd="true"
        type="editable-card" @edit="closeTerminalTab" @change="onTabChange">
        <!-- 主页标签页 - 服务器管理 -->
        <a-tab-pane key="home" tab="主页" :closable="false">
          <a-layout class="layout">
            <a-layout-sider width="270" class="sider">
              <div class="group-section">
                <div class="group-header">
                  <h3>服务器分组</h3>
                  <a-button type="primary" size="small" @click="showAddGroupModal">+</a-button>
                </div>
                <a-menu :selectedKeys="selectedGroupKeys" mode="inline" @select="onGroupSelect">
                  <a-menu-item v-for="group in groups" :key="group.id">
                    <template #icon>
                      <folder-outlined />
                    </template>
                    {{ group.name }}
                    <span class="group-actions">
                      <edit-outlined @click.stop="editGroup(group)" />
                      <delete-outlined @click.stop="deleteGroup(group.id)" />
                    </span>
                  </a-menu-item>
                </a-menu>
              </div>
            </a-layout-sider>

            <a-layout>
              <a-layout-content class="content">
                <div class="server-header">
                  <a-button v-if="currentGroupId" type="primary" @click="showAddServerModal">
                    <plus-outlined /> 添加服务器
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
                          {{ record.connected ? '断开' : '连接' }}
                        </a-button>
                        <a-button size="small" :disabled="!record.connected" @click="openTerminal(record)">终端</a-button>
                        <a-button size="small" :disabled="!record.connected" @click="manageFiles(record)">文件</a-button>
                        <a-button size="small" :disabled="record.connected" @click="editServer(record)">编辑</a-button>
                        <a-button size="small" :disabled="record.connected" @click="deleteServer(record)">删除</a-button>
                      </a-space>
                    </template>
                  </template>
                </a-table>
              </a-layout-content>
            </a-layout>
          </a-layout>
        </a-tab-pane>

        <!-- 批量脚本标签页 -->
        <a-tab-pane key="batch-script" tab="批量脚本" :closable="false">
          <BatchScriptManager />
        </a-tab-pane>

        <!-- 终端标签页 -->
        <a-tab-pane v-for="tab in terminalTabs" :key="tab.id" :tab="tab.title" closable>
          <div v-if="tab.type === 'terminal'" style="height: 100%; background: #1e1e1e; padding: 0; margin: 0">
            <Terminal :server="tab.server" :server-id="tab.serverId" @terminal-ready="checkPendingScript" />
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
  </div>
</template>

<script>
import {
  DeleteOutlined,
  DownOutlined,
  EditOutlined,
  FolderOutlined,
  PlusOutlined
} from '@ant-design/icons-vue';
import {
  AddServer,
  AddServerGroup,
  CloseTerminalSession,
  ConnectToServer,
  DeleteServer,
  DeleteServerGroup,
  DisconnectFromServer,
  GetServerGroups,
  UpdateServer,
  UpdateServerGroup,
  HandleFileUploadRequest,
  HandleFileDownloadRequest,
  SendScriptToTerminal // 添加导入
} from '../../wailsjs/go/controllers/SSHController';
import Terminal from './Terminal.vue';
import FileManager from './FileManager.vue';
import BatchScriptManager from './BatchScriptManager.vue';

export default {
  name: 'ServerManager',
  components: {
    FolderOutlined,
    EditOutlined,
    DeleteOutlined,
    PlusOutlined,
    DownOutlined,
    Terminal,
    FileManager,
    BatchScriptManager
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
      pendingScript: null, // 添加待处理脚本的存储

      // 分组模态框
      groupModalVisible: false,
      editingGroup: null,
      groupForm: {
        name: ''
      },

      // 服务器模态框
      serverModalVisible: false,
      editingServer: null,
      authMethod: 'password',
      serverForm: {
        name: '',
        host: '',
        port: 22,
        username: 'root',
        password: '',
        keyFile: '',
        note: ''
      },

      serverColumns: [
        { title: '服务器名称', dataIndex: 'name', key: 'name' },
        { title: '主机地址', dataIndex: 'host', key: 'host' },
        { title: '端口', dataIndex: 'port', key: 'port' },
        { title: '状态', dataIndex: 'status', key: 'status' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    };
  },
  async mounted() {
    await this.loadServerGroups();
    // 添加对自定义事件的监听
    window.addEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal);
  },
  
  beforeUnmount() {
    // 移除事件监听
    window.removeEventListener('execute-script-in-terminal', this.handleExecuteScriptInTerminal);
  },

  methods: {
    async loadServerGroups() {
      try {
        this.groups = await GetServerGroups();
        this.groups.forEach(group => {
          group.servers.forEach(server => {
            server.connected = false;
          });
        });
        this.closedSessions.clear();
      } catch (error) {
        console.error('加载服务器分组失败:', error);
      }
    },

    onGroupSelect(info) {
      const selectedKeys = info.selectedKeys;
      if (selectedKeys.length > 0) {
        const groupId = selectedKeys[0];
        this.selectedGroupKeys = [groupId];
        this.currentGroupId = groupId;

        const group = this.groups.find(g => g.id === groupId);
        if (group) {
          this.currentGroupName = group.name;
          this.currentServers = [...group.servers];
        }
      }
    },

    showAddGroupModal() {
      this.editingGroup = null;
      this.groupForm.name = '';
      this.groupModalVisible = true;
    },

    editGroup(group) {
      this.editingGroup = group;
      this.groupForm.name = group.name;
      this.groupModalVisible = true;
    },

    async handleGroupModalOk() {
      if (!this.groupForm.name.trim()) {
        this.$message.warning('请输入分组名称');
        return;
      }

      try {
        const groupData = {
          id: this.editingGroup ? this.editingGroup.id : 'group_' + Date.now(),
          name: this.groupForm.name,
          servers: this.editingGroup ? this.editingGroup.servers : []
        };

        if (this.editingGroup) {
          await UpdateServerGroup(groupData);
        } else {
          await AddServerGroup(groupData);
        }

        this.groupModalVisible = false;
        await this.loadServerGroups();
        this.$message.success(`${this.editingGroup ? '更新' : '添加'}分组成功`);
      } catch (error) {
        console.error(`${this.editingGroup ? '更新' : '添加'}分组失败:`, error);
        this.$message.error(`${this.editingGroup ? '更新' : '添加'}分组失败: ${error.message}`);
      }
    },

    async deleteGroup(groupId) {
      try {
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除',
            content: '确定要删除这个分组吗？分组内的服务器也将被删除。',
            okText: '确认',
            cancelText: '取消',
            onOk: () => resolve(),
            onCancel: () => reject('cancel')
          });
        });

        await DeleteServerGroup(groupId);
        await this.loadServerGroups();
        this.$message.success('删除分组成功');
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除分组失败:', error);
          this.$message.error(`删除分组失败: ${error.message}`);
        }
      }
    },

    showAddServerModal() {
      this.editingServer = null;
      this.serverForm = {
        name: '',
        host: '',
        port: 22,
        username: '',
        password: '',
        keyFile: '',
        note: ''
      };
      this.authMethod = 'password';
      this.serverModalVisible = true;
    },

    editServer(server) {
      this.editingServer = server;
      this.serverForm = { ...server };
      this.authMethod = server.keyFile ? 'key' : 'password';
      this.serverModalVisible = true;
    },

    async handleServerModalOk() {
      const form = this.serverForm;
      if (
        !form.name?.trim() ||
        !form.host?.trim() ||
        !form.username?.trim() ||
        (this.authMethod === 'password' && !form.password?.trim()) ||
        (this.authMethod === 'key' && !form.keyFile?.trim())
      ) {
        this.$message.warning('请填写所有必填字段');
        return;
      }

      try {
        const serverData = {
          id: this.editingServer ? this.editingServer.id : 'server_' + Date.now(),
          name: form.name,
          host: form.host,
          port: form.port,
          username: form.username,
          password: this.authMethod === 'password' ? form.password : '',
          keyFile: this.authMethod === 'key' ? form.keyFile : '',
          note: form.note || '',
          groupId: this.currentGroupId
        };

        if (this.editingServer) {
          await UpdateServer(this.currentGroupId, serverData);
        } else {
          await AddServer(this.currentGroupId, serverData);
        }

        this.serverModalVisible = false;
        await this.loadServerGroups();
        const group = this.groups.find(g => g.id === this.currentGroupId);
        if (group) {
          this.currentServers = [...group.servers];
        }
        this.$message.success(`${this.editingServer ? '更新' : '添加'}服务器成功`);
      } catch (error) {
        console.error(`${this.editingServer ? '更新' : '添加'}服务器失败:`, error);
        this.$message.error(`${this.editingServer ? '更新' : '添加'}服务器失败: ${error.message}`);
      }
    },

    async deleteServer(server) {
      try {
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除',
            content: '确定要删除这个服务器吗？',
            okText: '确认',
            cancelText: '取消',
            onOk: () => resolve(),
            onCancel: () => reject('cancel')
          });
        });

        await DeleteServer(server.groupId, server.id);
        await this.loadServerGroups();
        const group = this.groups.find(g => g.id === this.currentGroupId);
        if (group) {
          this.currentServers = [...group.servers];
        }
        this.$message.success('删除服务器成功');
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除服务器失败:', error);
          this.$message.error(`删除服务器失败: ${error.message}`);
        }
      }
    },

    async connectServer(server) {
      try {
        server.loading = true;
        if (server.connected) {
          const result = await DisconnectFromServer(server.id);
          server.connected = false;
          if (result && !result.includes('EOF') && !result.includes('断开')) {
            this.$message.success(result);
          }
          this.closedSessions.delete(server.id);
        } else {
          const result = await ConnectToServer(server.id);
          server.connected = true;
          this.$message.success(result);
        }
        server.loading = false;
      } catch (error) {
        server.loading = false;
        console.error('连接/断开服务器失败:', error);
        if (server.connected) {
          if (error.message && !error.message.includes('EOF') && !error.message.includes('断开')) {
            this.$message.error(`断开服务器连接失败: ${error.message}`);
          }
        } else {
          this.$message.error(`连接服务器失败: ${error.message}`);
        }
      }
    },

    async openTerminal(server) {
      const existingTab = this.terminalTabs.find(
        tab => tab.serverId === server.id && tab.type === 'terminal'
      );
      if (existingTab) {
        this.activeKey = existingTab.id;
        return;
      }

      if (!server.connected) {
        try {
          const result = await ConnectToServer(server.id);
          server.connected = true;
          this.$message.success(result);
        } catch (error) {
          console.error('连接服务器失败:', error);
          this.$message.error(`连接服务器失败: ${error.message}`);
          return;
        }
      }

      this.closedSessions.delete(server.id);

      const tabId = `terminal_${server.id}`;
      const newTab = {
        id: tabId,
        serverId: server.id,
        server: server,
        title: server.name,
        type: 'terminal'
      };
      this.terminalTabs.push(newTab);
      this.$nextTick(() => {
        this.activeKey = newTab.id;
      });
    },

    async closeTerminalTab(targetKey, action) {
      if (action !== 'remove') return;

      const tabIndex = this.terminalTabs.findIndex(tab => tab.id === targetKey);
      if (tabIndex === -1) return;

      const tab = this.terminalTabs[tabIndex];

      if (!this.closedSessions.has(tab.serverId)) {
        this.closedSessions.add(tab.serverId);
        try {
          const result = await CloseTerminalSession(tab.serverId);
          console.log(`终端会话 ${tab.serverId} 已关闭: ${result}`);
        } catch (error) {
          console.error('关闭终端会话失败:', error);
          if (error.message && !error.message.includes('EOF') && !error.message.includes('断开')) {
            this.$message.error(`关闭终端会话失败: ${error.message}`);
          }
        }
      }

      this.$nextTick(() => {
        if (!this.$el) return;
        this.terminalTabs.splice(tabIndex, 1);
        this.activeKey = this.terminalTabs.length > 0 ? this.terminalTabs[0].id : 'home';
      });
    },

    onTabChange(activeKey) {
      this.activeKey = activeKey;
    },

    manageFiles(server) {
      const existingTab = this.terminalTabs.find(
        tab => tab.serverId === server.id && tab.type === 'file'
      );
      if (existingTab) {
        this.activeKey = existingTab.id;
        return;
      }

      if (!server.connected) {
        this.$message.warning('请先连接服务器');
        return;
      }

      const tabId = `file_${server.id}`;
      const newTab = {
        id: tabId,
        serverId: server.id,
        server: server,
        title: `${server.name} - 文件管理`,
        type: 'file'
      };

      this.terminalTabs.push(newTab);
      this.$nextTick(() => {
        this.activeKey = tabId;
      });
    },

    // 处理终端执行脚本的请求
    async handleExecuteScriptInTerminal(event) {
      const script = event.detail.script;
      
      // 选择服务器（这里选择第一个服务器，实际应用中可能需要用户选择）
      if (script.serverIds.length === 0) {
        this.$message.error('脚本没有关联的服务器');
        return;
      }
      
      const serverId = script.serverIds[0]; // 选择第一个服务器
      
      // 查找服务器信息
      let server = null;
      for (const group of this.groups) {
        const found = group.servers.find(s => s.id === serverId);
        if (found) {
          server = found;
          break;
        }
      }
      
      if (!server) {
        this.$message.error('找不到指定的服务器');
        return;
      }
      
      // 确保服务器已连接
      if (!server.connected) {
        try {
          const result = await ConnectToServer(server.id);
          server.connected = true;
          this.$message.success(result);
        } catch (error) {
          console.error('连接服务器失败:', error);
          this.$message.error(`连接服务器失败: ${error.message}`);
          return;
        }
      }
      
      // 检查是否已经存在对应的终端标签页
      const existingTab = this.terminalTabs.find(
        tab => tab.serverId === server.id && tab.type === 'terminal'
      );
      
      if (existingTab) {
        // 如果已存在终端标签页，激活它并重新发送脚本命令
        this.activeKey = existingTab.id;
        
        // 存储脚本信息，等待终端准备就绪后发送命令
        this.pendingScript = {
          script: script,
          serverId: serverId
        };
        
        // 直接尝试发送脚本到终端（适用于终端已经完全准备好的情况）
        setTimeout(() => {
          this.sendScriptToTerminal(script, serverId);
          this.pendingScript = null;
        }, 1500);
        
        return;
      }
      
      // 打开终端窗口
      this.openTerminal(server);
      
      // 存储脚本信息，等待终端准备就绪后发送命令
      this.pendingScript = {
        script: script,
        serverId: serverId
      };
    },
    
    // 在终端组件挂载后检查是否有待处理的脚本
    checkPendingScript(serverId) {
      if (this.pendingScript && this.pendingScript.serverId === serverId) {
        // 延迟一段时间确保终端完全准备好
        setTimeout(() => {
          this.sendScriptToTerminal(this.pendingScript.script, serverId);
          this.pendingScript = null;
        }, 1000);
      }
    },
    
    // 发送脚本命令到终端
    async sendScriptToTerminal(script, serverId) {
      // 解析脚本命令
      const commands = script.content.split('\n').filter(cmd => cmd.trim() !== '');
      
      // 逐行发送命令
      for (const command of commands) {
        // 忽略注释和空行
        if (command.trim() === '' || command.trim().startsWith('#')) {
          continue;
        }
        
        // 检查是否是文件上传命令
        if (command.trim().startsWith('$upload ')) {
          // 文件上传命令，调用实际的上传逻辑
          const uploadParams = command.trim().substring(8).trim().split(/\s+/);
          if (uploadParams.length >= 2) {
            const localPath = uploadParams[0];
            const remotePath = uploadParams[1];
            
            try {
              // 调用后端的文件上传方法
              await HandleFileUploadRequest(serverId, localPath, remotePath);
              console.log(`文件上传完成: ${localPath} -> ${remotePath}`);
            } catch (error) {
              console.error('文件上传失败:', error);
            }
          } else {
            console.error('上传命令格式错误:', command);
          }
          // 等待文件操作完成后再继续执行后续命令
          await new Promise(resolve => setTimeout(resolve, 1000));
          continue;
        }
        
        // 检查是否是文件下载命令
        if (command.trim().startsWith('$download ')) {
          // 文件下载命令，调用实际的下载逻辑
          const downloadParams = command.trim().substring(10).trim().split(/\s+/);
          if (downloadParams.length >= 2) {
            const remotePath = downloadParams[0];
            const localPath = downloadParams[1];
            
            try {
              // 调用后端的文件下载方法
              await HandleFileDownloadRequest(serverId, remotePath, localPath);
              console.log(`文件下载完成: ${remotePath} -> ${localPath}`);
            } catch (error) {
              console.error('文件下载失败:', error);
            }
          } else {
            console.error('下载命令格式错误:', command);
          }
          // 等待文件操作完成后再继续执行后续命令
          await new Promise(resolve => setTimeout(resolve, 1000));
          continue;
        }
        
        // 发送普通命令到终端
        // 通过全局事件总线发送命令
        const event = new CustomEvent('send-command-to-terminal', { 
          detail: { serverId: serverId, command: command } 
        });
        window.dispatchEvent(event);
        
        // 等待一小段时间，模拟用户输入间隔
        await new Promise(resolve => setTimeout(resolve, 500));
      }
      
      this.$message.success('脚本命令处理完成');
    }
  }
};
</script>

<style scoped>
.main-tabs-container {
  height: 100%;
}

.layout {
  height: 100%;
}

.sider {
  background: #fff;
  border-right: 1px solid #f0f0f0;
}

.group-section {
  padding: 16px;
}

.group-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.group-header h3 {
  margin: 0;
}

.group-actions {
  float: right;
}

.group-actions .anticon {
  margin-left: 8px;
  cursor: pointer;
}

.content {
  padding: 16px;
  background: #fff;
}
</style>