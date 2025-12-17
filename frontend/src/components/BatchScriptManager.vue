<template>
  <div class="batch-script-manager">
    <div class="script-header">
      <a-button type="primary" @click="showAddScriptModal">
        <PlusOutlined /> 新建脚本
      </a-button>
      <a-input-search v-model:value="searchKeyword" placeholder="搜索脚本" style="width: 200px; margin-left: 16px"
        @search="onSearch" />
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

    <!-- 执行结果模态框 -->
    <a-modal v-model:open="executionResultVisible" title="执行结果" width="1200px" :footer="null">
      <div class="execution-results">
        <div v-for="result in executionResults" :key="result.serverId" class="result-item">
          <div class="result-header">
            <h4>{{ result.serverName }}</h4>
            <a-tag :color="getStatusColor(result.status)">
              {{ getStatusText(result.status) }}
            </a-tag>
          </div>
          <div v-if="result.startTime" class="result-time">
            开始时间: {{ result.startTime }}
          </div>

          <!-- 如果有整体错误信息，优先显示 -->
          <div v-if="result.error" class="result-error">
            <strong>执行错误:</strong>
            <pre>{{ result.error }}</pre>
          </div>

          <!-- 分命令显示执行结果 -->
          <div v-if="result.commandOutputs && result.commandOutputs.length > 0" class="command-results">
            <div class="command-results-header">
              <strong>命令执行详情:</strong>
            </div>
            <div v-for="(cmdResult, index) in result.commandOutputs" :key="index" class="command-item">
              <div class="command-header">
                <a-tag :color="cmdResult.status === 'success' ? 'green' : 'red'" size="small">
                  {{ cmdResult.status === 'success' ? '成功' : '失败' }}
                </a-tag>
                <code class="command-text">{{ cmdResult.command }}</code>
                <span class="command-time">{{ cmdResult.startTime }} - {{ cmdResult.endTime }}</span>
              </div>

              <div v-if="cmdResult.output" class="command-output">
                <strong>输出:</strong>
                <pre>{{ cmdResult.output }}</pre>
              </div>
              <div v-if="cmdResult.error" class="command-error">
                <strong>错误:</strong>
                <pre>{{ cmdResult.error }}</pre>
              </div>
              <!-- 如果命令失败但没有独立的错误信息，则在输出中显示错误 -->
              <div v-else-if="cmdResult.status === 'failed' && cmdResult.output" class="command-error">
                <strong>错误:</strong>
                <pre>{{ cmdResult.output }}</pre>
              </div>
            </div>
          </div>
          <!-- 兼容旧的输出格式 -->
          <div v-else-if="result.output" class="result-output">
            <strong>输出:</strong>
            <pre>{{ result.output }}</pre>
          </div>
        </div>
      </div>
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
} from '../../wailsjs/go/controllers/SSHController';
import {
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  SelectOutlined,
  CodeOutlined
} from '@ant-design/icons-vue';

export default {
  name: 'BatchScriptManager',
  components: {
    PlusOutlined,
    SelectOutlined,
    EditOutlined,
    DeleteOutlined,
    CodeOutlined
  },
  data() {
    return {
      scripts: [],
      serverGroups: [],
      selectedServerIds: [],
      searchKeyword: '',
      scriptModalVisible: false,
      executionResultVisible: false,
      editingScript: null,
      executingScriptId: '',
      executionResults: [],
      scriptForm: {
        name: '',
        description: '',
        content: '',
        executionType: 'command', // 默认为命令模式
        serverIds: []
      },
      scriptColumns: [
        { title: '目标服务器', dataIndex: 'serverIds', key: 'serverIds' },
        { title: '脚本名称', dataIndex: 'name', key: 'name' },
        { title: '执行类型', dataIndex: 'executionType', key: 'executionType' },
        { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    };
  },
  computed: {
    filteredScripts() {
      if (!this.searchKeyword) {
        return this.scripts;
      }
      const keyword = this.searchKeyword.toLowerCase();
      return this.scripts.filter(script =>
        script.name.toLowerCase().includes(keyword) ||
        script.description.toLowerCase().includes(keyword) ||
        script.content.toLowerCase().includes(keyword)
      );
    },
    serverOptions() {
      return this.serverGroups.map(group => ({
        label: group.name,
        title: group.name,
        options: group.servers.map(server => ({
          label: `${server.name} (${server.host}:${server.port})`,
          value: server.id,
          title: `${server.name} - ${server.host}:${server.port}`
        }))
      }));
    }
  },
  async mounted() {
    await this.loadScripts();
    await this.loadServerGroups();
  },
  methods: {
    async loadScripts() {
      try {
        this.scripts = await GetBatchScripts();
      } catch (error) {
        console.error('加载脚本失败:', error);
        this.$message.error('加载脚本失败: ' + error.message);
      }
    },

    async loadServerGroups() {
      try {
        this.serverGroups = await GetServerGroups();
      } catch (error) {
        console.error('加载服务器分组失败:', error);
        this.$message.error('加载服务器分组失败: ' + error.message);
      }
    },

    getServerName(serverId) {
      for (const group of this.serverGroups) {
        const server = group.servers.find(s => s.id === serverId);
        if (server) {
          return server.name;
        }
      }
      return '未知服务器';
    },

    getContentPreview(content) {
      if (!content) return '';
      return content.length > 50 ? content.substring(0, 50) + '...' : content;
    },

    onSearch() {
      // 搜索逻辑已经在计算属性中处理
    },

    showAddScriptModal() {
      this.editingScript = null;
      this.scriptForm = {
        name: '',
        description: '',
        content: '',
        executionType: 'command', // 默认为命令模式
        serverIds: []
      };
      this.selectedServerIds = [];
      this.scriptModalVisible = true;
    },

    editScript(script) {
      this.editingScript = script;
      const serverIds = Array.isArray(script.serverIds) ? script.serverIds : [];
      this.scriptForm = {
        name: script.name,
        description: script.description,
        content: script.content,
        executionType: script.executionType || 'command', // 兼容旧数据
        serverIds: [...serverIds]
      };
      this.selectedServerIds = [...serverIds];
      this.scriptModalVisible = true;
    },



    async handleScriptModalOk() {
      if (!this.scriptForm.name.trim()) {
        this.$message.warning('请输入脚本名称');
        return;
      }
      if (!this.scriptForm.content.trim()) {
        this.$message.warning('请输入脚本内容');
        return;
      }

      // 确保 selectedServerIds 是数组
      const serverIds = Array.isArray(this.selectedServerIds) ? this.selectedServerIds : [];
      if (serverIds.length === 0) {
        this.$message.warning('请选择至少一个目标服务器');
        return;
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
        };

        if (this.editingScript) {
          await UpdateBatchScript(scriptData);
        } else {
          await AddBatchScript(scriptData);
        }

        this.scriptModalVisible = false;
        await this.loadScripts();
        this.$message.success(`${this.editingScript ? '更新' : '创建'}脚本成功`);
      } catch (error) {
        console.error(`${this.editingScript ? '更新' : '创建'}脚本失败:`, error);
        this.$message.error(`${this.editingScript ? '更新' : '创建'}脚本失败: ${error.message}`);
      }
    },

    async deleteScript(script) {
      try {
        await DeleteBatchScript(script.id);
        await this.loadScripts();
        this.$message.success('删除脚本成功');
      } catch (error) {
        console.error('删除脚本失败:', error);
        this.$message.error('删除脚本失败: ' + error.message);
      }
    },

    async executeScript(script) {
      this.executingScriptId = script.id;
      try {
        const results = await ExecuteBatchScript(script.id);
        console.log('执行结果详情:', JSON.stringify(results, null, 2));
        this.executionResults = Object.values(results);
        this.executionResultVisible = true;
        this.$message.success('脚本执行完成');
      } catch (error) {
        console.error('执行脚本失败:', error);
        this.$message.error('执行脚本失败: ' + error.message);
      } finally {
        this.executingScriptId = '';
      }
    },

    // 终端执行脚本方法
    executeScriptInTerminal(script) {
      // 触发自定义事件，传递脚本信息
      // 让ServerManager来处理终端打开和命令发送
      const event = new CustomEvent('execute-script-in-terminal', {
        detail: { script: script }
      });
      window.dispatchEvent(event);
    },

    getStatusColor(status) {
      switch (status) {
        case 'success': return 'green';
        case 'failed': return 'red';
        case 'running': return 'blue';
        case 'pending': return 'orange';
        default: return 'gray';
      }
    },

    getStatusText(status) {
      switch (status) {
        case 'success': return '成功';
        case 'failed': return '失败';
        case 'running': return '执行中';
        case 'pending': return '等待中';
        default: return '未知';
      }
    },

    getContentPlaceholder(executionType) {
      if (executionType === 'script') {
        return '请输入完整的Shell脚本内容\n示例：\n#!/bin/bash\necho "开始执行脚本"\nfor i in {1..5}\ndo\n  echo "循环 $i"\ndone\n\n# 文件操作会自动处理\n$upload ./config.json /etc/myapp/\n$download /var/log/app.log ./logs/\necho "文件操作完成"';
      } else {
        return '请输入要执行的Shell命令\n示例：\necho "Hello World"\nls -la\npwd\n\n# 文件操作示例\n$upload ./dist.tar.gz /tmp/\n$download /var/backup/db.sql ./backup/';
      }
    },

    getContentHelp(executionType) {
      if (executionType === 'script') {
        return '<strong>脚本模式：</strong>整个脚本将作为Shell脚本执行，支持文件操作。当脚本包含文件操作时会自动切换到混合执行模式。<br><strong>文件操作：</strong>支持 $upload 本地路径 远程路径 和 $download 远程路径 本地路径';
      } else {
        return '<strong>命令模式：</strong>每行命令将单独执行，遇到失败命令时停止后续执行。适合执行独立的命令序列。<br><strong>文件操作：</strong>支持 $upload 本地路径 远程路径 和 $download 远程路径 本地路径';
      }
    },

    getExecutionTypeText(executionType) {
      switch (executionType) {
        case 'script': return '脚本模式';
        case 'command': return '命令模式';
        default: return '命令模式'; // 兼容旧数据
      }
    }
  }
};
</script>

<style scoped>
.batch-script-manager {
  padding: 16px;
}

.script-header {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
}

.content-preview {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  color: #666;
}

.script-help {
  margin-top: 8px;
  color: #999;
}

.server-help {
  margin-top: 4px;
  color: #999;
}

.execution-results {
  max-height: 500px;
  overflow-y: auto;
}

.result-item {
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  padding: 12px;
  margin-bottom: 12px;
}

.result-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.result-header h4 {
  margin: 0;
}

.result-time {
  color: #666;
  font-size: 12px;
  margin-bottom: 8px;
}

.result-output,
.result-error {
  margin-top: 8px;
}

.result-output pre,
.result-error pre {
  background: #f5f5f5;
  padding: 8px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-wrap: break-word;
  margin: 4px 0 0 0;
  font-family: 'Courier New', monospace;
  font-size: 12px;
}

.result-error pre {
  background: #fff2f0;
  color: #a8071a;
}

.command-results {
  margin-top: 12px;
  border: 1px solid #e8e8e8;
  border-radius: 4px;
  background: #fafafa;
}

.command-results-header {
  padding: 8px 12px;
  background: #f0f0f0;
  border-bottom: 1px solid #e8e8e8;
  font-weight: bold;
}

.command-item {
  border-bottom: 1px solid #f0f0f0;
  padding: 12px;
}

.command-item:last-child {
  border-bottom: none;
}

.command-header {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
  gap: 8px;
}

.command-text {
  background: #f6f8fa;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: 'Courier New', monospace;
  font-size: 13px;
  color: #d73a49;
  border: 1px solid #e1e4e8;
  flex: 1;
}

.command-time {
  font-size: 11px;
  color: #666;
  white-space: nowrap;
}

.command-output,
.command-error {
  margin-left: 24px;
  margin-top: 6px;
}

.command-output pre,
.command-error pre {
  background: #f6f8fa;
  padding: 8px;
  border-radius: 4px;
  white-space: pre-wrap;
  word-wrap: break-word;
  margin: 4px 0 0 0;
  font-family: 'Courier New', monospace;
  font-size: 12px;
  border-left: 3px solid #28a745;
}

.command-error pre {
  background: #fff5f5;
  color: #d73a49;
  border-left-color: #cb2431;
}

.execution-type-help {
  margin-top: 8px;
  color: #666;
  line-height: 1.4;
}
</style>