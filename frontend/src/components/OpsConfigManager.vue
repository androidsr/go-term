<template>
  <div class="ops-config-manager">
    <div class="ops-header">
      <a-button type="primary" @click="showAddModal">
        <PlusOutlined /> 新建运维配置
      </a-button>
      <a-input-search
        v-model:value="searchKeyword"
        placeholder="搜索配置名称 / 应用目录 / 备注"
        style="width: 280px; margin-left: 16px"
        allow-clear
      />
    </div>

    <a-table
      :dataSource="filteredConfigs"
      :columns="columns"
      :pagination="{ pageSize: 10 }"
      rowKey="id"
      size="small"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'serverName'">
          <a-tag color="blue">{{ getServerName(record.serverId) }}</a-tag>
        </template>
        <template v-else-if="column.dataIndex === 'deployMethod'">
          <a-tag :color="deployMethodColor(record.deployMethod)">{{ deployMethodText(record.deployMethod) }}</a-tag>
        </template>
        <template v-else-if="column.dataIndex === 'appDir'">
          <code class="dir-code">{{ record.appDir || '-' }}</code>
        </template>
        <template v-else-if="column.dataIndex === 'action'">
          <a-space>
            <a-button size="small" @click="copyDeployCmd(record)">复制部署信息</a-button>
            <a-button size="small" type="primary" @click="editConfig(record)">编辑</a-button>
            <a-popconfirm title="确定删除该配置？" ok-text="确认" cancel-text="取消" @confirm="deleteConfig(record)">
              <a-button size="small" danger>删除</a-button>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-modal
      v-model:open="modalVisible"
      :title="editing ? '编辑运维配置' : '新建运维配置'"
      width="720px"
      @ok="handleOk"
      @cancel="modalVisible = false"
    >
      <a-form :model="form" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="配置名称" required>
              <a-input v-model:value="form.name" placeholder="例如：生产-订单服务" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="关联服务器" required>
              <a-select v-model:value="form.serverId" placeholder="选择服务器" :options="serverOptions" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="16">
          <a-col :span="12">
            <a-form-item label="部署方式">
              <a-select v-model:value="form.deployMethod" placeholder="选择部署方式">
                <a-select-option value="source">源码编译</a-select-option>
                <a-select-option value="binary">二进制部署</a-select-option>
                <a-select-option value="docker">Docker</a-select-option>
                <a-select-option value="docker-compose">Docker Compose</a-select-option>
                <a-select-option value="k8s">Kubernetes</a-select-option>
                <a-select-option value="systemd">Systemd 服务</a-select-option>
                <a-select-option value="other">其他</a-select-option>
              </a-select>
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="代码仓库">
              <a-input v-model:value="form.repoUrl" placeholder="git 仓库地址（可选）" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="应用所在目录">
          <a-input v-model:value="form.appDir" placeholder="例如：/opt/myapp 或 /home/ubuntu/app" />
        </a-form-item>
        <a-form-item label="启动命令 / 启动脚本路径">
          <a-input v-model:value="form.startCmd" placeholder="例如：/opt/myapp/start.sh 或 systemctl start myapp" />
        </a-form-item>
        <a-form-item label="日志目录">
          <a-input v-model:value="form.logsDir" placeholder="例如：/opt/myapp/logs（可选）" />
        </a-form-item>
        <a-form-item label="环境变量">
          <a-textarea v-model:value="form.envVars" :rows="3" placeholder="KEY=VALUE，每行一个（可选）" />
        </a-form-item>
        <a-form-item label="备注">
          <a-textarea v-model:value="form.notes" :rows="2" placeholder="部署注意事项、回滚步骤等（可选）" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script>
import {
  GetOpsConfigs,
  AddOpsConfig,
  UpdateOpsConfig,
  DeleteOpsConfig,
  GetServerGroups
} from '../../bindings/go-term/controllers/sshcontroller'
import { PlusOutlined } from '@ant-design/icons-vue'

export default {
  name: 'OpsConfigManager',
  components: {
    PlusOutlined
  },
  data() {
    return {
      configs: [],
      serverGroups: [],
      searchKeyword: '',
      modalVisible: false,
      editing: null,
      form: this.emptyForm(),
      columns: [
        { title: '名称', dataIndex: 'name', key: 'name' },
        { title: '服务器', dataIndex: 'serverName', key: 'serverName' },
        { title: '部署方式', dataIndex: 'deployMethod', key: 'deployMethod' },
        { title: '应用目录', dataIndex: 'appDir', key: 'appDir' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ]
    }
  },
  computed: {
    filteredConfigs() {
      const kw = (this.searchKeyword || '').toLowerCase()
      if (!kw) return this.configs
      return this.configs.filter(
        (c) =>
          (c.name || '').toLowerCase().includes(kw) ||
          (c.appDir || '').toLowerCase().includes(kw) ||
          (c.notes || '').toLowerCase().includes(kw)
      )
    },
    serverOptions() {
      return this.serverGroups.map((g) => ({
        label: g.name,
        title: g.name,
        options: g.servers.map((s) => ({
          label: `${s.name} (${s.host}:${s.port})`,
          value: s.id
        }))
      }))
    }
  },
  async mounted() {
    await this.loadConfigs()
    await this.loadServers()
  },
  methods: {
    emptyForm() {
      return {
        id: '',
        name: '',
        serverId: undefined,
        deployMethod: 'binary',
        repoUrl: '',
        appDir: '',
        startCmd: '',
        logsDir: '',
        envVars: '',
        notes: ''
      }
    },
    async loadConfigs() {
      try {
        this.configs = await GetOpsConfigs()
      } catch (e) {
        console.error('加载运维配置失败', e)
        this.$message.error('加载运维配置失败: ' + e.message)
      }
    },
    async loadServers() {
      try {
        this.serverGroups = await GetServerGroups()
      } catch (e) {
        console.error('加载服务器失败', e)
      }
    },
    getServerName(serverId) {
      for (const g of this.serverGroups) {
        const s = g.servers.find((x) => x.id === serverId)
        if (s) return s.name
      }
      return serverId || '未关联'
    },
    deployMethodText(v) {
      const map = {
        source: '源码编译',
        binary: '二进制部署',
        docker: 'Docker',
        'docker-compose': 'Docker Compose',
        k8s: 'Kubernetes',
        systemd: 'Systemd',
        other: '其他'
      }
      return map[v] || v || '-'
    },
    deployMethodColor(v) {
      const map = {
        source: 'purple',
        binary: 'blue',
        docker: 'cyan',
        'docker-compose': 'geekblue',
        k8s: 'gold',
        systemd: 'green',
        other: 'default'
      }
      return map[v] || 'default'
    },
    showAddModal() {
      this.editing = null
      this.form = this.emptyForm()
      this.modalVisible = true
    },
    editConfig(cfg) {
      this.editing = cfg
      this.form = Object.assign(this.emptyForm(), cfg)
      this.modalVisible = true
    },
    async handleOk() {
      if (!this.form.name.trim()) {
        this.$message.warning('请填写配置名称')
        return
      }
      if (!this.form.serverId) {
        this.$message.warning('请选择关联服务器')
        return
      }
      const payload = Object.assign({}, this.form)
      try {
        if (this.editing) {
          await UpdateOpsConfig(payload)
        } else {
          await AddOpsConfig(payload)
        }
        this.modalVisible = false
        await this.loadConfigs()
        this.$message.success('保存成功')
      } catch (e) {
        console.error('保存失败', e)
        this.$message.error('保存失败: ' + e.message)
      }
    },
    async deleteConfig(cfg) {
      try {
        await DeleteOpsConfig(cfg.id)
        await this.loadConfigs()
        this.$message.success('已删除')
      } catch (e) {
        console.error('删除失败', e)
        this.$message.error('删除失败: ' + e.message)
      }
    },
    copyDeployCmd(cfg) {
      const lines = [
        `服务器: ${this.getServerName(cfg.serverId)}`,
        `部署方式: ${this.deployMethodText(cfg.deployMethod)}`,
        `应用目录: ${cfg.appDir || '-'}`,
        `启动命令: ${cfg.startCmd || '-'}`,
        `日志目录: ${cfg.logsDir || '-'}`,
        `环境变量:\n${cfg.envVars || '-'}`,
        `备注: ${cfg.notes || '-'}`
      ]
      const text = lines.join('\n')
      navigator.clipboard.writeText(text).then(
        () => this.$message.success('部署信息已复制到剪贴板'),
        () => this.$message.error('复制失败')
      )
    }
  }
}
</script>

<style scoped>
.ops-config-manager {
  padding: 16px;
}
.ops-header {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 10px;
}
.dir-code {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  background: rgba(24, 144, 255, 0.08);
  padding: 2px 6px;
  border-radius: 3px;
}
</style>
