<template>
  <div class="ops-config-manager">
    <div class="ops-header">
      <a-button type="primary" @click="showAddModal">
        <PlusOutlined /> 新建配置
      </a-button>
      <a-input-search
        v-model:value="searchKeyword"
        placeholder="搜索名称 / 内容"
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
        <template v-else-if="column.dataIndex === 'action'">
          <a-space>
            <a-button size="small" @click="viewConfig(record)">查看</a-button>
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
      :title="modalTitle"
      :width="'min(920px, 94vw)'"
      :footer="readonly ? null : undefined"
      @ok="handleOk"
      @cancel="modalVisible = false"
    >
      <a-form :model="form" layout="vertical">
        <a-row :gutter="16">
          <a-col :span="14">
            <a-form-item label="配置名称" required>
              <a-input
                v-model:value="form.name"
                :disabled="readonly"
                placeholder="例如：生产-订单服务"
              />
            </a-form-item>
          </a-col>
          <a-col :span="10">
            <a-form-item label="关联项目（服务器）">
              <a-select
                v-model:value="form.serverId"
                :disabled="readonly"
                placeholder="选择项目（可留空）"
                :allowClear="true"
                :options="serverOptions"
              />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="内容（自由文本）">
          <a-textarea
            v-model:value="form.content"
            :disabled="readonly"
            :autosize="{ minRows: 16, maxRows: 30 }"
            style="font-family: 'Courier New', Consolas, monospace; font-size: 13px"
            placeholder="在此输入任意运维配置内容：部署步骤、命令、注意事项、回滚方案……"
          />
        </a-form-item>
      </a-form>
      <div v-if="readonly" style="margin-top: 8px">
        <a-button size="small" @click="copyContent(form)">
          <CopyOutlined /> 复制内容
        </a-button>
      </div>
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
import { PlusOutlined, CopyOutlined } from '@ant-design/icons-vue'

export default {
  name: 'OpsConfigManager',
  components: {
    PlusOutlined,
    CopyOutlined
  },
  data() {
    return {
      configs: [],
      serverGroups: [],
      searchKeyword: '',
      modalVisible: false,
      editing: null,
      readonly: false,
      form: this.emptyForm(),
      columns: [
        { title: '名称', dataIndex: 'name', key: 'name' },
        { title: '项目', dataIndex: 'serverName', key: 'serverName' },
        { title: '更新时间', dataIndex: 'updatedAt', key: 'updatedAt' },
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
          (c.content || '').toLowerCase().includes(kw)
      )
    },
    serverOptions() {
      const base = [{ label: '通用配置（不关联服务器）', value: '' }]
      const groups = this.serverGroups.map((g) => ({
        label: g.name,
        title: g.name,
        options: g.servers.map((s) => ({
          label: `${s.name} (${s.host}:${s.port})`,
          value: s.id
        }))
      }))
      return base.concat(groups)
    },
    modalTitle() {
      if (this.readonly) return '查看运维配置'
      return this.editing ? '编辑运维配置' : '新建运维配置'
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
        content: ''
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
      if (!serverId) return '通用'
      for (const g of this.serverGroups) {
        const s = g.servers.find((x) => x.id === serverId)
        if (s) return s.name
      }
      return serverId
    },
    showAddModal() {
      this.editing = null
      this.readonly = false
      this.form = this.emptyForm()
      this.modalVisible = true
    },
    editConfig(cfg) {
      this.editing = cfg
      this.readonly = false
      this.form = Object.assign(this.emptyForm(), cfg)
      this.modalVisible = true
    },
    viewConfig(cfg) {
      this.editing = cfg
      this.readonly = true
      this.form = Object.assign(this.emptyForm(), cfg)
      this.modalVisible = true
    },
    async handleOk() {
      if (!this.form.name.trim()) {
        this.$message.warning('请填写配置名称')
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
    copyContent(cfg) {
      navigator.clipboard.writeText(cfg.content || '').then(
        () => this.$message.success('内容已复制到剪贴板'),
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
</style>
