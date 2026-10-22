<template>
  <div class="ai-page">
    <!-- 顶部：目标服务器与操作 -->
    <div class="ai-toolbar">
      <span class="label">目标服务器：</span>
      <a-select v-model:value="targetServerId" style="width: 280px" placeholder="选择已连接的服务器" size="small"
        :disabled="running">
        <a-select-option v-for="s in connectedServers" :key="s.id" :value="s.id">
          {{ s.name }} ({{ s.host }})
        </a-select-option>
      </a-select>
      <a-button size="small" @click="loadServers" title="刷新服务器列表">
        <ReloadOutlined />
      </a-button>
      <a-tooltip title="清除 AI 对该服务器的历史任务记忆">
        <a-button size="small" :disabled="!targetServerId" @click="clearMemory">
          <HistoryOutlined />清空记忆
        </a-button>
      </a-tooltip>
      <span v-if="!aiEnabled" class="ai-warn">⚠ AI 功能未启用，请到「设置 → AI 模型」配置</span>
      <a-button v-if="messages.length && !running" size="small" class="clear-btn" @click="clearMessages">
        <ClearOutlined />清空记录
      </a-button>
    </div>

    <!-- 中部：对话显示区域 -->
    <div class="ai-messages" ref="messagesEl">
      <div v-if="!messages.length" class="ai-empty">
        <RobotOutlined style="font-size: 44px; color: #555" />
        <p>用自然语言描述任务，AI 将自动探测服务器环境、逐步执行命令并实时展示结果。<br />AI 会记住该服务器最近几次任务的内容与结论，可以直接说「继续上次的任务」。<br />例如：「帮我安装 nginx 并启动，设置开机自启」</p>
      </div>

      <template v-for="msg in messages" :key="msg.id">
        <!-- 用户任务 -->
        <div v-if="msg.kind === 'user'" class="ai-msg user">
          <div class="ai-msg-tag">任务</div>
          <div class="ai-msg-body">{{ msg.text }}</div>
        </div>

        <!-- AI 说明文字（渲染 Markdown） -->
        <div v-else-if="msg.kind === 'ai'" class="ai-msg ai">
          <div class="ai-msg-tag">AI</div>
          <div class="ai-msg-body md" :class="{ streaming: msg.streaming }" v-html="renderMarkdown(msg.text)"></div>
        </div>

        <!-- 执行的命令 -->
        <div v-else-if="msg.kind === 'command'" class="ai-msg cmd">
          <div class="ai-msg-tag">执行</div>
          <div class="ai-msg-body">
            <div class="cmd-line"><code>{{ msg.command }}</code>
              <a-tag v-if="msg.exitCode !== undefined" :color="msg.exitCode === 0 ? 'green' : 'red'" class="cmd-tag">
                exit {{ msg.exitCode }}
              </a-tag>
              <LoadingOutlined v-else spin style="color: #69b1ff" />
            </div>
            <pre v-if="msg.output" class="cmd-output">{{ msg.output }}<span v-if="msg.running" class="cursor">▍</span></pre>
          </div>
        </div>

        <!-- 状态 -->
        <div v-else-if="msg.kind === 'status'" class="ai-status">{{ msg.text }}</div>

        <!-- 最终总结（渲染 Markdown） -->
        <div v-else-if="msg.kind === 'final'" class="ai-msg final">
          <div class="ai-msg-tag">总结</div>
          <div class="ai-msg-body md" v-html="renderMarkdown(msg.text)"></div>
        </div>

        <!-- 错误 -->
        <div v-else-if="msg.kind === 'error'" class="ai-msg error">
          <div class="ai-msg-tag">错误</div>
          <div class="ai-msg-body">{{ msg.text }}</div>
        </div>
      </template>
    </div>

    <!-- 底部：对话输入与发送区域 -->
    <div class="ai-input-bar">
      <a-textarea v-model:value="taskInput" class="ai-textarea" :rows="3"
        placeholder="描述你要完成的任务，例如：帮我安装 nginx 并启动，设置开机自启（Enter 发送，Shift+Enter 换行）"
        @keydown.enter.exact.prevent="startTask" :disabled="running" />
      <div class="ai-send">
        <a-button v-if="!running" type="primary" :disabled="!taskInput.trim() || !targetServerId"
          @click="startTask">
          <RocketOutlined /> 开始执行
        </a-button>
        <a-button v-else danger @click="stopTask">
          <StopOutlined /> 停止任务
        </a-button>
        <div v-if="running" class="ai-status-hint">{{ statusText }}</div>
      </div>
    </div>
  </div>
</template>

<script>
import { Events } from '@wailsio/runtime'
import { message } from 'ant-design-vue'
import {
  GetAIConfig,
  GetAIConfiguredServers,
  StartAITask,
  StopAITask,
  ClearAIMemory
} from '../../bindings/go-term/controllers/aicontroller'
import {
  RobotOutlined, RocketOutlined, StopOutlined, ReloadOutlined, LoadingOutlined,
  ClearOutlined, HistoryOutlined
} from '@ant-design/icons-vue'

let msgSeq = 0
const OUTPUT_LIMIT = 64 * 1024 // 单命令输出展示上限

// 轻量 Markdown 渲染：代码块 / 行内代码 / 加粗 / 标题 / 列表，全部先转义防注入
function renderMarkdown(src) {
  if (!src) return ''
  const blocks = []
  let text = String(src).replace(/```[\w-]*\n?([\s\S]*?)```/g, (m, code) => {
    blocks.push(code)
    return '@@BLOCK' + (blocks.length - 1) + '@@'
  })
  // 流式输出中未闭合的代码块
  text = text.replace(/```[\w-]*\n?([\s\S]*)$/, (m, code) => {
    blocks.push(code)
    return '@@BLOCK' + (blocks.length - 1) + '@@'
  })
  text = text
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/^\s*#{1,6}\s+(.+)$/gm, '<strong class="md-h">$1</strong>')
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/^\s*[-*]\s+(.+)$/gm, '<span class="md-li">• $1</span>')
    .replace(/\n/g, '<br>')
  text = text.replace(/@@BLOCK(\d+)@@/g, (m, i) => {
    const code = blocks[+i] || ''
    return '<pre class="md-pre">' +
      code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
      '</pre>'
  })
  return text
}

export default {
  name: 'AIAssistant',
  components: { RobotOutlined, RocketOutlined, StopOutlined, ReloadOutlined, LoadingOutlined, ClearOutlined, HistoryOutlined },
  data() {
    return {
      connectedServers: [],
      targetServerId: undefined,
      taskInput: '',
      running: false,
      currentTaskId: null,
      statusText: '',
      messages: [],
      aiEnabled: false,
      // 内部引用，避免频繁触发响应式
      _currentAiMsg: null,
      _currentCmdMsg: null
    }
  },
  mounted() {
    this.loadServers()
    this.refreshAiEnabled()
    Events.On('ai-event', (event) => this.handleAiEvent(event.data))
  },
  beforeUnmount() {
    Events.Off('ai-event')
  },
  watch: {
    // 向外广播任务运行状态（供主界面 Tab 徽标显示），所有状态变更路径都会经过这里
    running(val) {
      window.dispatchEvent(new CustomEvent('ai-task-status', { detail: { running: val } }))
    }
  },
  methods: {
    async loadServers() {
      try {
        this.connectedServers = (await GetAIConfiguredServers()) || []
        if (this.targetServerId && !this.connectedServers.some((s) => s.id === this.targetServerId)) {
          this.targetServerId = undefined
        }
        if (!this.targetServerId && this.connectedServers.length === 1) {
          this.targetServerId = this.connectedServers[0].id
        }
      } catch (e) {
        this.connectedServers = []
      }
    },
    async refreshAiEnabled() {
      try {
        const cfg = await GetAIConfig()
        this.aiEnabled = !!cfg.enabled
      } catch (e) { /* ignore */ }
    },
    async startTask() {
      const task = (this.taskInput || '').trim()
      if (!task || !this.targetServerId) return
      this.messages = []
      this._currentAiMsg = null
      this._currentCmdMsg = null
      this.running = true
      this.statusText = '任务启动中…'
      this.pushMessage('user', task)
      try {
        await StartAITask(this.targetServerId, task)
        this.taskInput = ''
      } catch (e) {
        this.running = false
        this.pushMessage('error', e.message || String(e))
      }
    },
    async stopTask() {
      if (!this.currentTaskId) return
      try {
        await StopAITask(this.currentTaskId)
        message.info('已发送停止指令')
      } catch (e) {
        message.error(`停止失败: ${e.message}`)
      }
    },
    clearMessages() {
      this.messages = []
      this._currentAiMsg = null
      this._currentCmdMsg = null
      this.refreshAiEnabled()
    },
    async clearMemory() {
      if (!this.targetServerId) return
      try {
        await ClearAIMemory(this.targetServerId)
        message.success('已清空该服务器的任务记忆')
      } catch (e) {
        message.error(`清空记忆失败: ${e.message}`)
      }
    },
    renderMarkdown,
    pushMessage(kind, text, extra = {}) {
      const m = { id: ++msgSeq, kind, text, ...extra }
      this.messages.push(m)
      this.$nextTick(this.scrollToBottom)
      return m
    },
    ensureAiMsg() {
      if (!this._currentAiMsg) {
        this._currentAiMsg = this.pushMessage('ai', '', { streaming: true })
      }
      return this._currentAiMsg
    },
    ensureCmdMsg(command) {
      this._currentAiMsg = null
      this._currentCmdMsg = this.pushMessage('command', '', {
        command, output: '', running: true
      })
      return this._currentCmdMsg
    },
    handleAiEvent(data) {
      if (!data || typeof data !== 'object') return
      if (this.currentTaskId && data.taskID !== this.currentTaskId) return

      switch (data.type) {
        case 'start':
          this.currentTaskId = data.taskID
          // 显示任务实际使用的接口配置，便于确认配置加载正确
          if (data.endpoint || data.model) {
            this.pushMessage('status', `接口: ${data.endpoint || '-'} · 模型: ${data.model || '-'}`)
          }
          break
        case 'status':
          this.statusText = data.message || ''
          break
        case 'delta': {
          const m = this.ensureAiMsg()
          m.text += data.text || ''
          this.scrollToBottom()
          break
        }
        case 'command': {
          this.statusText = ''
          this.ensureCmdMsg(data.command)
          break
        }
        case 'output': {
          const m = this._currentCmdMsg
          if (m) {
            m.output = (m.output + (data.chunk || ''))
            if (m.output.length > OUTPUT_LIMIT) {
              m.output = m.output.slice(-OUTPUT_LIMIT)
            }
            this.scrollToBottom()
          }
          break
        }
        case 'command-done': {
          const m = this._currentCmdMsg
          if (m) {
            m.running = false
            m.exitCode = data.exitCode
            if (data.error) m.output += `\n[异常] ${data.error}`
            this._currentCmdMsg = null
          }
          break
        }
        case 'final': {
          this._currentAiMsg = null
          this.pushMessage('final', data.summary || '(无总结)')
          break
        }
        case 'error':
          this._currentAiMsg = null
          this.pushMessage('error', data.message || '发生未知错误')
          break
        case 'done':
          this.running = false
          this.currentTaskId = null
          this.statusText = ''
          break
      }
    },
    scrollToBottom() {
      const el = this.$refs.messagesEl
      if (!el) return
      // 用户手动上滚查看历史时不强制拉底，靠近底部时才跟随
      const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 120
      if (nearBottom) el.scrollTop = el.scrollHeight
    }
  }
}
</script>

<style scoped>
.ai-page {
  height: 100%; display: flex; flex-direction: column;
  padding: 12px 16px 16px; gap: 10px; box-sizing: border-box; overflow: hidden;
}

/* 顶部工具行 */
.ai-toolbar { display: flex; align-items: center; gap: 8px; flex-shrink: 0; }
.ai-toolbar .label { color: #b8c7d9; font-size: 13px; white-space: nowrap; }
.ai-warn { color: #faad14; font-size: 12.5px; }
.clear-btn { margin-left: auto; }

/* 中部显示区域 */
.ai-messages {
  flex: 1; min-height: 0; overflow-y: auto; border: 1px solid #333;
  border-radius: 8px; padding: 14px 16px; background: rgba(0, 0, 0, 0.25);
  font-size: 13.5px;
}
.ai-empty { text-align: center; color: #8c8c8c; padding-top: 120px; line-height: 2.2; }

.ai-msg { margin-bottom: 14px; }
.ai-msg-tag {
  display: inline-block; font-size: 12px; padding: 1px 8px; border-radius: 3px;
  margin-bottom: 5px; font-weight: 600;
}
.ai-msg.user .ai-msg-tag { background: #164e63; color: #7dd3fc; }
.ai-msg.ai .ai-msg-tag { background: #14532d; color: #86efac; }
.ai-msg.cmd .ai-msg-tag { background: #713f12; color: #fde047; }
.ai-msg.final .ai-msg-tag { background: #1e3a8a; color: #93c5fd; }
.ai-msg.error .ai-msg-tag { background: #7f1d1d; color: #fecaca; }

.ai-msg-body {
  font-size: 13.5px; line-height: 1.75; white-space: pre-wrap; word-break: break-word;
  color: #ececec;
}
.ai-msg.user .ai-msg-body { color: #cffafe; }
.ai-msg.error .ai-msg-body { color: #fca5a5; }
.ai-msg.final .ai-msg-body { color: #dbeafe; }

/* Markdown 渲染元素（换行已转为 <br>，关闭 pre-wrap 防止双倍行距） */
.ai-msg-body.md { white-space: normal; }
.ai-msg-body :deep(code) {
  background: #262626; color: #ffd591; padding: 1px 6px; border-radius: 4px;
  font-family: Consolas, Monaco, monospace; font-size: 12.5px; white-space: pre-wrap;
}
.ai-msg-body :deep(strong) { color: #ffffff; font-weight: 600; }
.ai-msg-body :deep(.md-h) { display: inline-block; margin-top: 2px; }
.ai-msg-body :deep(.md-li) { display: inline-block; margin-left: 2px; }
.ai-msg-body :deep(.md-pre) {
  display: block; margin: 6px 0; padding: 10px 12px;
  background: #0d1117; border: 1px solid #30363d; border-radius: 6px;
  color: #c9d1d9; font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  line-height: 1.6; white-space: pre-wrap; word-break: break-all;
  max-height: 300px; overflow-y: auto;
}

.cmd-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.cmd-line code {
  background: #1a1a1a; border: 1px solid #333; padding: 5px 10px; border-radius: 4px;
  color: #ffe58f; font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  flex: 1; min-width: 0; white-space: pre-wrap; word-break: break-all;
}
.cmd-tag { margin: 0; }
.cmd-output {
  margin: 6px 0 0; padding: 10px 12px; background: #0d1117;
  border: 1px solid #30363d; border-radius: 4px;
  color: #d6deeb; font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  line-height: 1.6; max-height: 280px; overflow-y: auto;
  white-space: pre-wrap; word-break: break-all;
}

.ai-status { color: #a3b8cc; font-size: 12.5px; text-align: center; margin: 4px 0; }
.cursor { animation: blink 1s infinite; color: #69b1ff; }
@keyframes blink { 50% { opacity: 0; } }

/* 底部输入区域 */
.ai-input-bar {
  flex-shrink: 0; display: flex; gap: 10px; align-items: flex-end;
}
.ai-textarea { flex: 1; font-size: 13.5px; }
.ai-send {
  display: flex; flex-direction: column; align-items: stretch; gap: 6px; min-width: 110px;
}
.ai-send :deep(.ant-btn) { width: 100%; }
.ai-status-hint { color: #69b1ff; font-size: 12px; text-align: center; }
</style>
