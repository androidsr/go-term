<template>
  <div class="batch-script-manager">
    <div class="script-header">
      <a-button type="primary" @click="showAddScriptModal">
        <PlusOutlined /> 新建脚本
      </a-button>
      <a-input-search v-model:value="searchKeyword" placeholder="搜索脚本" style="width: 200px; margin-left: 16px"
        @search="onSearch" />
      <a-badge :count="taskStore.scripts.length" :offset="[-4, 2]" style="margin-left: 16px">
        <a-button @click="taskStore.openDrawer('script')">
          <UnorderedListOutlined /> 任务中心
        </a-button>
      </a-badge>
    </div>

    <a-table :dataSource="filteredScripts" :columns="scriptColumns" :pagination="{ pageSize: 10 }" rowKey="id" size="small">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'content'">
          <a-tooltip :title="record.content">
            <span class="content-preview">{{ getContentPreview(record.content) }}</span>
          </a-tooltip>
        </template>
        <template v-else-if="column.dataIndex === 'executionType'">
          <a-tag :color="record.executionType === 'script' ? 'green' : 'blue'" size="small">
            {{ getExecutionTypeText(record.executionType) }}
          </a-tag>
        </template>
        <template v-else-if="column.dataIndex === 'serverIds'">
          <a-tag v-for="serverId in record.serverIds" :key="serverId" color="blue" size="small">
            {{ getServerName(serverId) }}
          </a-tag>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <a-space>
            <a-button size="small" type="primary"
              @click="record.executionType === 'script' ? executeScript(record) : executeScriptInTerminal(record)"
              :loading="executingScriptId === record.id">
              <CodeOutlined />执行
            </a-button>
            <a-button size="small" @click="editScript(record)">
              <EditOutlined />编辑
            </a-button>
            <a-popconfirm title="确定要删除这个脚本吗？" ok-text="确认" cancel-text="取消" @confirm="deleteScript(record)">
              <a-button size="small" danger>
                <DeleteOutlined />删除
              </a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <!-- 添加/编辑脚本模态框 -->
    <a-modal v-model:open="scriptModalVisible" :title="editingScript ? '编辑脚本' : '新建脚本'" width="800px"
      @ok="handleScriptModalOk" @cancel="scriptModalVisible = false">
      <a-form :model="scriptForm" layout="vertical">
        <a-form-item label="脚本名称" required>
          <a-input v-model:value="scriptForm.name" placeholder="请输入脚本名称" />
        </a-form-item>
        <a-form-item label="目标服务器" required>
          <a-select v-model:value="selectedServerIds" mode="multiple" placeholder="请选择目标服务器" :options="serverOptions"
            :field-names="{ label: 'label', value: 'value', options: 'options' }" style="width: 100%">
            <template #suffixIcon>
              <select-outlined />
            </template>
          </a-select>
          <div class="server-help">
            <small>支持多选，可按住 Ctrl 键进行多选</small>
          </div>
        </a-form-item>
        <a-form-item label="执行类型" required>
          <a-radio-group v-model:value="scriptForm.executionType" button-style="solid">
            <a-radio-button value="command">命令模式</a-radio-button>
            <a-radio-button value="script">脚本模式</a-radio-button>
          </a-radio-group>
          <div class="execution-type-help">
            <small>
              <strong>命令模式：</strong>逐条执行每条命令，遇到失败命令时停止执行<br>
              <strong>脚本模式：</strong>将整个内容作为完整脚本执行，保持脚本上下文
            </small>
          </div>
        </a-form-item>
        <a-form-item label="脚本内容" required>
          <a-textarea v-model:value="scriptForm.content" :placeholder="getContentPlaceholder(scriptForm.executionType)"
            :rows="10" style="font-family: 'Courier New', monospace;" />
          <div class="script-help">
            <small v-html="getContentHelp(scriptForm.executionType)"></small>
          </div>
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script>
import {
  AddBatchScript,
  DeleteBatchScript,
  ExecuteBatchScript,
  GetBatchScripts,
  UpdateBatchScript,
  GetServerGroups
} from '../../bindings/go-term/controllers/sshcontroller'
import { Events } from '@wailsio/runtime'
import {
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  SelectOutlined,
  CodeOutlined,
  UnorderedListOutlined
} from '@ant-design/icons-vue'
import { taskStore, newUID } from '../store/taskStore.js'

export default {
  name: 'BatchScriptManager',
  components: {
    PlusOutlined,
    SelectOutlined,
    EditOutlined,
    DeleteOutlined,
    CodeOutlined,
    UnorderedListOutlined
  },
  data() {
    return {
      scripts: [],
      serverGroups: [],
      selectedServerIds: [],
      searchKeyword: '',
      scriptModalVisible: false,
      editingScript: null,
      executingScriptId: '',
      scriptTaskMap: {}, // scriptID -> taskStore 任务ID
      scriptForm: {
        name: '',
        description: '',
        content: '',
        executionType: 'command',
        serverIds: []
      },
      scriptColumns: [
        { title: '目标服务器', dataIndex: 'serverIds', key: 'serverIds' },
        { title: '脚本名称', dataIndex: 'name', key: 'name' },
        { title: '执行类型', dataIndex: 'executionType', key: 'executionType' },
        { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    }
  },
  computed: {
    taskStore() {
      return taskStore
    },
    filteredScripts() {
      if (!this.searchKeyword) return this.scripts
      const keyword = this.searchKeyword.toLowerCase()
      return this.scripts.filter((script) =>
        script.name.toLowerCase().includes(keyword) ||
        (script.description || '').toLowerCase().includes(keyword) ||
        script.content.toLowerCase().includes(keyword)
      )
    },
    serverOptions() {
      return this.serverGroups.map((group) => ({
        label: group.name,
        title: group.name,
        options: group.servers.map((server) => ({
          label: `${server.name} (${server.host}:${server.port})`,
          value: server.id,
          title: `${server.name} - ${server.host}:${server.port}`
        }))
      }))
    }
  },
  async mounted() {
    await this.loadScripts()
    await this.loadServerGroups()
    Events.On('script-task-created', (event) => this.onScriptTaskCreated(event.data))
    Events.On('script-task-update', (event) => this.onScriptTaskUpdate(event.data))
  },
  beforeUnmount() {
    Events.Off('script-task-created')
    Events.Off('script-task-update')
  },
  methods: {
    async loadScripts() {
      try {
        this.scripts = await GetBatchScripts()
      } catch (error) {
        console.error('加载脚本失败:', error)
        this.$message.error('加载脚本失败: ' + error.message)
      }
    },
    async loadServerGroups() {
      try {
        this.serverGroups = await GetServerGroups()
      } catch (error) {
        console.error('加载服务器分组失败:', error)
        this.$message.error('加载服务器分组失败: ' + error.message)
      }
    },
    getServerName(serverId) {
      for (const group of this.serverGroups) {
        const server = group.servers.find((s) => s.id === serverId)
        if (server) return server.name
      }
      return '未知服务器'
    },
    getContentPreview(content) {
      if (!content) return ''
      return content.length > 50 ? content.substring(0, 50) + '...' : content
    },
    onSearch() {},
    showAddScriptModal() {
      this.editingScript = null
      this.scriptForm = { name: '', description: '', content: '', executionType: 'command', serverIds: [] }
      this.selectedServerIds = []
      this.scriptModalVisible = true
    },
    editScript(script) {
      this.editingScript = script
      const serverIds = Array.isArray(script.serverIds) ? script.serverIds : []
      this.scriptForm = {
        name: script.name,
        description: script.description,
        content: script.content,
        executionType: script.executionType || 'command',
        serverIds: [...serverIds]
      }
      this.selectedServerIds = [...serverIds]
      this.scriptModalVisible = true
    },
    async handleScriptModalOk() {
      if (!this.scriptForm.name.trim()) {
        this.$message.warning('请输入脚本名称')
        return
      }
      if (!this.scriptForm.content.trim()) {
        this.$message.warning('请输入脚本内容')
        return
      }
      const serverIds = Array.isArray(this.selectedServerIds) ? this.selectedServerIds : []
      if (serverIds.length === 0) {
        this.$message.warning('请选择至少一个目标服务器')
        return
      }
      try {
        const scriptData = {
          id: this.editingScript ? this.editingScript.id : 'script_' + Date.now(),
          name: this.scriptForm.name,
          description: this.scriptForm.description,
          content: this.scriptForm.content,
          executionType: this.scriptForm.executionType,
          serverIds: [...serverIds],
          createdAt: this.editingScript ? this.editingScript.createdAt : '',
          updatedAt: ''
        }
        if (this.editingScript) {
          await UpdateBatchScript(scriptData)
        } else {
          await AddBatchScript(scriptData)
        }
        this.scriptModalVisible = false
        await this.loadScripts()
        this.$message.success(`${this.editingScript ? '更新' : '创建'}脚本成功`)
      } catch (error) {
        console.error(`${this.editingScript ? '更新' : '创建'}脚本失败:`, error)
        this.$message.error(`${this.editingScript ? '更新' : '创建'}脚本失败: ${error.message}`)
      }
    },
    async deleteScript(script) {
      try {
        await DeleteBatchScript(script.id)
        await this.loadScripts()
        this.$message.success('删除脚本成功')
      } catch (error) {
        console.error('删除脚本失败:', error)
        this.$message.error('删除脚本失败: ' + error.message)
      }
    },

    // 后端批量执行：以任务列表 + 实时日志形式展示
    async executeScript(script) {
      this.executingScriptId = script.id
      const taskId = newUID('sc')
      this.scriptTaskMap[script.id] = taskId

      const servers = {}
      ;(script.serverIds || []).forEach((sid) => {
        servers[sid] = {
          serverID: sid,
          serverName: this.getServerName(sid),
          status: 'running',
          commandOutputs: [],
          error: ''
        }
      })
      taskStore.addScriptTask({
        id: taskId,
        scriptID: script.id,
        name: script.name,
        total: (script.serverIds || []).length,
        servers
      })
      taskStore.openDrawer('script')

      try {
        await ExecuteBatchScript(script.id)
      } catch (error) {
        console.error('执行脚本失败:', error)
        taskStore.updateScriptTask(taskId, { status: 'failed', error: error.message })
        this.$message.error('脚本执行失败: ' + error.message)
      } finally {
        this.executingScriptId = ''
      }
    },

    onScriptTaskCreated(data) {
      if (!data) return
      let taskId = this.scriptTaskMap[data.scriptID]
      if (!taskId) {
        taskId = newUID('sc')
        this.scriptTaskMap[data.scriptID] = taskId
      }
      const servers = {}
      const ids = data.serverIDs || []
      const names = data.serverNames || []
      ids.forEach((sid, i) => {
        servers[sid] = {
          serverID: sid,
          serverName: names[i] || sid,
          status: 'running',
          commandOutputs: [],
          error: ''
        }
      })
      taskStore.updateScriptTask(taskId, {
        id: taskId,
        scriptID: data.scriptID,
        name: data.scriptName,
        status: 'running',
        total: ids.length,
        done: 0,
        servers
      })
    },

    onScriptTaskUpdate(data) {
      if (!data) return
      const taskId = this.scriptTaskMap[data.scriptID]
      if (!taskId) return
      taskStore.updateScriptServer(taskId, data.serverID, {
        serverName: data.serverName,
        status: data.status,
        commandOutputs: data.commandOutputs || [],
        error: data.error || ''
      })
    },

    // 终端执行脚本方法
    executeScriptInTerminal(script) {
      const event = new CustomEvent('execute-script-in-terminal', { detail: { script } })
      window.dispatchEvent(event)
    },

    getStatusColor(status) {
      switch (status) {
        case 'success': return 'green'
        case 'failed': return 'red'
        case 'running': return 'blue'
        case 'pending': return 'orange'
        default: return 'gray'
      }
    },
    getStatusText(status) {
      switch (status) {
        case 'success': return '成功'
        case 'failed': return '失败'
        case 'running': return '执行中'
        case 'pending': return '等待中'
        default: return '未知'
      }
    },
    getContentPlaceholder(executionType) {
      if (executionType === 'script') {
        return '请输入完整的Shell脚本内容\n示例：\n#!/bin/bash\necho "开始执行脚本"\nfor i in {1..5}\ndo\n  echo "循环 $i"\ndone\n\n# 本地命令（以 ! 开头）在本地执行，不会发送到服务器\n!echo "这是本地命令"\n!dir\n\n# 文件操作会自动处理\n$upload ./config.json /etc/myapp/\n$download /var/log/app.log ./logs/\necho "文件操作完成"'
      } else {
        return '请输入要执行的Shell命令\n示例：\necho "Hello World"\nls -la\npwd\n\n# 本地命令（以 ! 开头）在本地执行，不会发送到服务器\n!echo "这是本地命令"\n!dir\n\n# 文件操作示例\n$upload ./dist.tar.gz /tmp/\n$download /var/backup/db.sql ./backup/'
      }
    },
    getContentHelp(executionType) {
      if (executionType === 'script') {
        return '<strong>脚本模式：</strong>整个脚本将作为Shell脚本执行，支持文件操作和本地命令。当脚本包含文件操作或本地命令时会自动切换到混合执行模式。<br><strong>文件操作：</strong>支持 $upload 本地路径 远程路径 和 $download 远程路径 本地路径<br><strong>本地命令：</strong>以 ! 开头的命令在本地执行，不会发送到服务器，如 !dir 或 !echo "hello"'
      } else {
        return '<strong>命令模式：</strong>每行命令将单独执行，遇到失败命令时停止后续执行。适合执行独立的命令序列。<br><strong>文件操作：</strong>支持 $upload 本地路径 远程路径 和 $download 远程路径 本地路径<br><strong>本地命令：</strong>以 ! 开头的命令在本地执行，不会发送到服务器，如 !dir 或 !echo "hello"'
      }
    },
    getExecutionTypeText(executionType) {
      switch (executionType) {
        case 'script': return '脚本模式'
        case 'command': return '命令模式'
        default: return '命令模式'
      }
    }
  }
}
</script>

<style scoped>
.batch-script-manager { padding: 16px; }
.script-header { display: flex; align-items: center; margin-bottom: 16px; flex-wrap: wrap; gap: 10px; }
.content-preview { font-family: 'Courier New', monospace; font-size: 12px; }
.script-help { margin-top: 8px; }
.server-help { margin-top: 4px; }
</style>
