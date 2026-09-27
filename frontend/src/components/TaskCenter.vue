<template>
  <a-drawer
    :open="taskStore.drawerVisible"
    :width="520"
    title="任务中心"
    placement="right"
    @close="taskStore.drawerVisible = false"
  >
    <a-tabs v-model:activeKey="taskStore.activeTab">
      <!-- 文件传输任务 -->
      <a-tab-pane key="transfer" :tab="`传输任务 (${taskStore.transfers.length})`">
        <a-empty v-if="taskStore.transfers.length === 0" description="暂无传输任务" />
        <a-list :dataSource="taskStore.transfers" item-layout="vertical" size="small">
          <template #renderItem="{ item }">
            <a-list-item>
              <div class="task-row">
                <div class="task-title">
                  <component
                    :is="item.type === 'upload' ? 'UploadOutlined' : 'DownloadOutlined'"
                    class="task-icon"
                  />
                  <span class="task-name" :title="item.name">{{ item.name }}</span>
                  <a-tag
                    :color="item.status === 'success' ? 'green' : item.status === 'error' ? 'red' : 'blue'"
                    size="small"
                  >
                    {{ item.status === 'success' ? '完成' : item.status === 'error' ? '失败' : '传输中' }}
                  </a-tag>
                </div>
                <div class="task-meta">
                  <span>{{ item.serverName || item.serverID }}</span>
                  <span v-if="item.status === 'running'">{{ item.speed }}</span>
                  <a-button
                    v-if="item.status !== 'running'"
                    type="link"
                    size="small"
                    danger
                    @click="taskStore.removeTransfer(item.id)"
                  >移除</a-button>
                </div>
                <a-progress
                  :percent="Math.floor(item.percent)"
                  size="small"
                  :status="item.status === 'error' ? 'exception' : item.status === 'success' ? 'success' : 'active'"
                />
                <div v-if="item.status === 'error'" class="task-error">{{ item.error }}</div>
              </div>
            </a-list-item>
          </template>
        </a-list>
        <div v-if="taskStore.transfers.length" class="task-footer">
          <a-button size="small" @click="taskStore.clearFinishedTransfers()">清除已完成</a-button>
        </div>
      </a-tab-pane>

      <!-- 批量脚本任务 -->
      <a-tab-pane key="script" :tab="`脚本任务 (${taskStore.scripts.length})`">
        <a-empty v-if="taskStore.scripts.length === 0" description="暂无脚本任务" />
        <a-list :dataSource="taskStore.scripts" item-layout="vertical" size="small">
          <template #renderItem="{ item }">
            <a-list-item>
              <div class="task-row">
                <div class="task-title">
                  <CodeOutlined class="task-icon" />
                  <span class="task-name" :title="item.name">{{ item.name }}</span>
                  <a-tag
                    :color="item.status === 'success' ? 'green' : item.status === 'failed' ? 'red' : 'blue'"
                    size="small"
                  >
                    {{ item.status === 'success' ? '成功' : item.status === 'failed' ? '失败' : '执行中' }}
                  </a-tag>
                </div>
                <div class="task-meta">
                  <span>{{ item.createdAt }}</span>
                  <span>完成 {{ item.done }}/{{ item.total }}</span>
                  <a-button type="link" size="small" danger @click="taskStore.removeScriptTask(item.id)">移除</a-button>
                </div>

                <a-collapse ghost :bordered="false" size="small">
                  <a-collapse-panel
                    v-for="srv in serverList(item)"
                    :key="srv.serverID"
                    :header="`${srv.serverName || srv.serverID} · ${srv.status === 'success' ? '成功' : srv.status === 'failed' ? '失败' : '执行中'}`"
                  >
                    <div v-if="srv.error" class="task-error">{{ srv.error }}</div>
                    <div
                      v-for="(cmd, idx) in srv.commandOutputs"
                      :key="idx"
                      class="log-block"
                    >
                      <div class="log-cmd">
                        <a-tag :color="cmd.status === 'success' ? 'green' : 'red'" size="small">
                          {{ cmd.status === 'success' ? '成功' : '失败' }}
                        </a-tag>
                        <code>{{ cmd.command }}</code>
                      </div>
                      <pre v-if="cmd.output" class="log-pre">{{ cmd.output }}</pre>
                      <pre v-if="cmd.error" class="log-pre error">{{ cmd.error }}</pre>
                    </div>
                    <a-empty v-if="!srv.commandOutputs || srv.commandOutputs.length === 0" :image="undefined" description="暂无日志" />
                  </a-collapse-panel>
                </a-collapse>
              </div>
            </a-list-item>
          </template>
        </a-list>
      </a-tab-pane>
    </a-tabs>
  </a-drawer>
</template>

<script>
import { UploadOutlined, DownloadOutlined, CodeOutlined } from '@ant-design/icons-vue'
import { taskStore } from '../store/taskStore.js'

export default {
  name: 'TaskCenter',
  components: {
    UploadOutlined,
    DownloadOutlined,
    CodeOutlined
  },
  data() {
    return {
      taskStore
    }
  },
  methods: {
    serverList(item) {
      return Object.values(item.servers || {})
    }
  }
}
</script>

<style scoped>
.task-row {
  width: 100%;
}
.task-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  margin-bottom: 4px;
}
.task-icon {
  color: #1890ff;
}
.task-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.task-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: #888;
  margin: 4px 0;
}
.task-error {
  color: #ff4d4f;
  font-size: 12px;
  margin-top: 4px;
  white-space: pre-wrap;
  word-break: break-all;
}
.task-footer {
  margin-top: 12px;
  text-align: right;
}
.log-block {
  margin-bottom: 8px;
}
.log-cmd {
  display: flex;
  align-items: center;
  gap: 6px;
}
.log-cmd code {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  word-break: break-all;
}
.log-pre {
  margin: 4px 0 0 0;
  padding: 8px;
  background: #1f1f1f;
  color: #e8e8e8;
  border-radius: 4px;
  font-family: 'Courier New', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow: auto;
}
.log-pre.error {
  color: #ff7875;
}
</style>
