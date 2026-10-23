<template>
  <div class="ai-page" :class="isLight ? 'theme-light' : 'theme-dark'">
    <div class="ai-main" ref="mainEl">
      <!-- 左：AI 对话区 -->
      <div class="ai-chat" :style="chatStyle">
        <div class="ai-messages" ref="messagesEl">
          <div v-if="!messages.length" class="ai-empty">
            <RobotOutlined style="font-size: 44px" />
            <p>用自然语言描述任务，AI 将自动探测服务器环境、逐步执行命令并实时展示结果。<br />
              AI 会记住该服务器最近几次任务的内容与结论，可以直接说「继续上次的任务」。<br />
              支持精确的文件编辑：「在 /etc/nginx/nginx.conf 的 server 块里加一段反向代理」。<br />
              例如：「帮我安装 nginx 并启动，设置开机自启」</p>
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
              <div class="ai-msg-body md" v-html="renderMarkdown(msg.text)"></div>
            </div>

            <!-- 执行的命令 -->
            <div v-else-if="msg.kind === 'command'" class="ai-msg cmd">
              <div class="ai-msg-tag">执行</div>
              <div class="ai-msg-body">
                <div class="cmd-line"><code>{{ msg.command }}</code>
                  <a-tag v-if="msg.exitCode !== undefined" :color="msg.exitCode === 0 ? 'green' : 'red'" class="cmd-tag">
                    exit {{ msg.exitCode }}
                  </a-tag>
                  <LoadingOutlined v-else spin style="color: #1677ff" />
                </div>
                <pre v-if="msg.output" class="cmd-output">{{ msg.output }}<span v-if="msg.running" class="cursor">▍</span></pre>
              </div>
            </div>

            <!-- 文件操作 -->
            <div v-else-if="msg.kind === 'file'" class="ai-msg file">
              <div class="ai-msg-tag">文件</div>
              <div class="ai-msg-body">
                <div class="cmd-line">
                  <FileTextOutlined class="file-icon" />
                  <code>{{ msg.title }}</code>
                  <a-tag v-if="msg.ok !== undefined" :color="msg.ok ? 'green' : 'red'" class="cmd-tag">
                    {{ msg.ok ? '成功' : '失败' }}
                  </a-tag>
                  <LoadingOutlined v-else spin style="color: #1677ff" />
                </div>
                <div v-if="msg.detail" class="file-detail">{{ msg.detail }}</div>
              </div>
            </div>

            <!-- 状态 -->
            <div v-else-if="msg.kind === 'status'" class="ai-status">{{ msg.text }}</div>

            <!-- 错误 -->
            <div v-else-if="msg.kind === 'error'" class="ai-msg error">
              <div class="ai-msg-tag">错误</div>
              <div class="ai-msg-body">{{ msg.text }}</div>
            </div>
          </template>
        </div>

        <!-- AI 未启用提醒（放输入区上方，不占头部） -->
        <div v-if="!aiEnabled" class="ai-warn-bar">⚠ AI 功能未启用，请到「设置 → AI 模型」配置后重试</div>

        <!-- 底部：一体化输入框（按钮浮动在输入框内底部） -->
        <div class="ai-composer">
          <a-textarea v-model:value="taskInput" class="ai-textarea" :rows="3"
            placeholder="描述你要完成的任务，Enter 发送 / Shift+Enter 换行"
            @keydown.enter.exact.prevent="startTask" :disabled="running" />
          <div class="composer-bar">
            <div class="left">
              <a-button v-if="messages.length && !running" size="small" type="text" class="ghost-btn"
                @click="clearMessages">
                <ClearOutlined />清空历史
              </a-button>
              <a-radio-group v-model:value="mode" size="small" button-style="solid" :disabled="running">
                <a-tooltip title="在后台 SSH 连接上执行命令，输出在对话区实时展示">
                  <a-radio-button value="exec">后台执行</a-radio-button>
                </a-tooltip>
                <a-tooltip title="AI 像人一样把命令逐条输入到右侧终端中，全程可见；执行期间终端输入锁定">
                  <a-radio-button value="interactive">终端交互</a-radio-button>
                </a-tooltip>
              </a-radio-group>
              <a-tooltip title="清除 AI 对该服务器的历史任务记忆">
                <a-button size="small" type="text" class="ghost-btn" @click="clearMemory">
                  <HistoryOutlined />清空记忆
                </a-button>
              </a-tooltip>
            </div>
            <div class="right">
              <span v-if="running" class="ai-status-hint">{{ statusText }}</span>
              <a-button v-if="!running" type="primary" shape="circle" class="send-btn"
                :disabled="!taskInput.trim() || !serverId" @click="startTask" title="发送（Enter）">
                <SendOutlined />
              </a-button>
              <a-tooltip v-else title="立即停止：中断正在执行的命令，AI 不再继续">
                <a-button type="primary" danger shape="circle" class="send-btn" @click="stopTask">
                  <StopOutlined />
                </a-button>
              </a-tooltip>
            </div>
          </div>
        </div>
      </div>

      <!-- 可拖拽分隔条：左右拖动调整对话区/终端区宽度，双击恢复对半 -->
      <div v-show="mode === 'interactive'" class="splitter" title="拖动调整宽度，双击恢复对半"
        @mousedown.prevent="startSplitDrag" @dblclick="chatPct = 50">
        <span class="splitter-grip"></span>
      </div>

      <!-- 右：交互模式终端区 -->
      <div v-show="mode === 'interactive'" class="ai-term">
        <Terminal v-if="aiSessionId" :session-id="aiSessionId" :server-id="serverId" :active="false" />
        <div v-else class="ai-term-placeholder">
          <LoadingOutlined v-if="!aiSessionFailed" spin style="font-size: 22px" />
          <span>{{ aiSessionFailed ? '终端会话创建失败' : '正在创建终端会话…' }}</span>
          <a-button v-if="aiSessionFailed" size="small" type="primary" @click="ensureSession(true)">重试</a-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { Events } from '@wailsio/runtime'
import { message } from 'ant-design-vue'
import {
  GetAIConfig,
  StartAITask,
  StopAITask,
  ClearAIMemory
} from '../../bindings/go-term/controllers/aicontroller'
import { CreateTerminalSessionWithSize } from '../../bindings/go-term/controllers/sshcontroller'
import { settingsStore } from '../store/settingsStore.js'
import Terminal from './Terminal.vue'
import {
  RobotOutlined, StopOutlined, SendOutlined, LoadingOutlined,
  ClearOutlined, HistoryOutlined, FileTextOutlined
} from '@ant-design/icons-vue'

let msgSeq = 0
const OUTPUT_LIMIT = 64 * 1024 // 单命令输出展示上限
const SPLIT_KEY = 'go-term-ai-split-pct' // 分栏宽度持久化

// 轻量 Markdown 渲染：标题/加粗/行内代码/代码块/列表/引用/分割线，全部先转义防注入
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
    .replace(/^\s*---+\s*$/gm, '<span class="md-hr"></span>')
    .replace(/^\s*&gt;\s?(.+)$/gm, '<span class="md-quote">$1</span>')
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/^\s*[-*]\s+(.+)$/gm, '<span class="md-li">• $1</span>')
    .replace(/^\s*(\d+)\.\s+(.+)$/gm, '<span class="md-li md-ol">$1. $2</span>')
    .replace(/\n/g, '<br>')
  text = text.replace(/@@BLOCK(\d+)@@/g, (m, i) => {
    const code = blocks[+i] || ''
    return '<pre class="md-pre">' +
      code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') +
      '</pre>'
  })
  return text
}

// 读取持久化的分栏比例（20%~80%，异常回退对半）
function loadSplitPct() {
  try {
    const v = Number(localStorage.getItem(SPLIT_KEY))
    if (v >= 20 && v <= 80) return v
  } catch (e) { /* ignore */ }
  return 50
}

// 从 AI 流式文本中清理协议指令行（RUN:/FINAL:），展示为正常对话
function stripDirectiveLines(text, stopAtFinal) {
  let t = String(text || '')
  if (stopAtFinal) {
    // 从 FINAL 行开始截断（其后内容作为总结单独渲染）
    t = t.replace(/^[ \t]*(?:```(?:shell|bash|sh)?[ \t]*)?FINAL[：:][\s\S]*$/gim, '')
  }
  t = t.replace(/^[ \t]*(?:```(?:shell|bash|sh)?[ \t]*)?RUN[：:][^\n]*$/gim, '')
  return t.replace(/\n{3,}/g, '\n\n')
}

export default {
  name: 'AIAssistant',
  components: {
    RobotOutlined, StopOutlined, SendOutlined, LoadingOutlined,
    ClearOutlined, HistoryOutlined, FileTextOutlined,
    Terminal
  },
  props: {
    serverId: { type: String, required: true },
    serverName: { type: String, default: '' }
  },
  data() {
    return {
      mode: 'exec',
      // 交互模式的专属终端会话：打开本 Tab 时自动创建，无需手动
      aiSessionId: null,
      aiSessionFailed: false,
      taskInput: '',
      running: false,
      currentTaskId: null,
      statusText: '',
      // 会话历史常驻显示（与后台记忆一致），仅超过最大轮数才裁掉最早的轮次
      messages: [],
      maxRounds: 10,
      aiEnabled: false,
      // 内部引用，避免频繁触发响应式
      _currentAiMsg: null,
      _currentCmdMsg: null,
      _currentFileMsg: null,
      // 左右分栏比例：对话区宽度百分比（可拖拽调整，持久化）
      chatPct: loadSplitPct()
    }
  },
  computed: {
    isLight() {
      return settingsStore.theme === 'light'
    },
    // 交互模式下对话区宽度跟随 chatPct，后台执行模式对话区占满
    chatStyle() {
      if (this.mode === 'interactive') {
        return { flex: '0 0 ' + this.chatPct + '%' }
      }
      return { flex: '1 1 auto' }
    }
  },
  mounted() {
    this.refreshAiEnabled()
    // 交互式终端默认创建一个，无需每次手动
    this.ensureSession()
    Events.On('ai-event', (event) => this.handleAiEvent(event.data))
    window.addEventListener('ai-tab-closing', this.handleTabClosing)
  },
  beforeUnmount() {
    Events.Off('ai-event')
    window.removeEventListener('ai-tab-closing', this.handleTabClosing)
  },
  watch: {
    // 向外广播任务运行状态（供 Tab 徽标显示），所有状态变更路径都会经过这里
    running(val) {
      window.dispatchEvent(new CustomEvent('ai-task-status', {
        detail: { running: val, serverId: this.serverId }
      }))
    },
    mode(val) {
      if (val === 'interactive') this.ensureSession()
    }
  },
  methods: {
    // Tab 被关闭：停止本服务器正在运行的任务
    handleTabClosing(e) {
      const sid = e.detail && e.detail.serverId
      if (sid === this.serverId && this.running && this.currentTaskId) {
        StopAITask(this.currentTaskId).catch(() => { /* ignore */ })
      }
    },
    // 确保（或强制重建）交互模式的终端会话；会话随 Terminal 组件卸载自动关闭
    async ensureSession(force) {
      if (this.aiSessionId && !force) return this.aiSessionId
      this.aiSessionId = null
      this.aiSessionFailed = false
      try {
        this.aiSessionId = await CreateTerminalSessionWithSize(this.serverId, 120, 30)
        return this.aiSessionId
      } catch (e) {
        this.aiSessionFailed = true
        return null
      }
    },
    // 锁定/解锁交互终端的键盘输入（AI 执行期间禁止手动操作）
    setTermLock(locked) {
      if (this.aiSessionId) {
        window.dispatchEvent(new CustomEvent('terminal-input-lock', {
          detail: { sessionId: this.aiSessionId, locked }
        }))
      }
    },
    async refreshAiEnabled() {
      try {
        const cfg = await GetAIConfig()
        this.aiEnabled = !!cfg.enabled
        this.maxRounds = cfg.memoryRounds > 0 ? cfg.memoryRounds : 10
      } catch (e) { /* ignore */ }
    },
    async startTask() {
      const task = (this.taskInput || '').trim()
      if (!task || !this.serverId) return
      if (this.mode === 'interactive' && !(await this.ensureSession())) {
        message.warning('终端交互模式需要终端会话，自动创建失败，请重试或改用后台执行')
        return
      }
      // 不清空历史：之前的会话记录保留在界面上（与后台记忆一致）
      this._currentAiMsg = null
      this._currentCmdMsg = null
      this._currentFileMsg = null
      this.running = true
      this.statusText = this.mode === 'interactive' ? '任务启动中（终端交互模式）…' : '任务启动中…'
      this.pushMessage('user', task)
      // 交互模式：AI 执行期间锁定右侧终端输入
      if (this.mode === 'interactive') this.setTermLock(true)
      try {
        await StartAITask(this.serverId, task, this.mode, this.aiSessionId || '')
        this.taskInput = ''
      } catch (e) {
        this.running = false
        this.setTermLock(false)
        this.pushMessage('error', e.message || String(e))
      }
    },
    async stopTask() {
      if (!this.currentTaskId) return
      this.statusText = '正在停止…'
      try {
        await StopAITask(this.currentTaskId)
      } catch (e) {
        message.error(`停止失败: ${e.message}`)
      }
    },
    clearMessages() {
      this.messages = []
      this._currentAiMsg = null
      this._currentCmdMsg = null
      this._currentFileMsg = null
      this.refreshAiEnabled()
    },
    async clearMemory() {
      if (!this.serverId) return
      try {
        await ClearAIMemory(this.serverId)
        message.success('已清空该服务器的任务记忆')
      } catch (e) {
        message.error(`清空记忆失败: ${e.message}`)
      }
    },
    // 任务结束后按最大轮数裁剪界面历史：只删最早的轮次，后面的保留
    trimHistory() {
      const max = this.maxRounds > 0 ? this.maxRounds : 0
      if (!max) return
      const userIdxs = []
      this.messages.forEach((m, i) => { if (m.kind === 'user') userIdxs.push(i) })
      if (userIdxs.length <= max) return
      const cutAt = userIdxs[userIdxs.length - max]
      this.messages = this.messages.slice(cutAt)
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
      // 命令开始前清理 AI 文本里的 RUN: 协议行，展示为正常对话
      if (this._currentAiMsg) {
        this._currentAiMsg.text = stripDirectiveLines(this._currentAiMsg.text, false)
        if (!this._currentAiMsg.text.trim()) {
          const idx = this.messages.indexOf(this._currentAiMsg)
          if (idx >= 0) this.messages.splice(idx, 1)
          this._currentAiMsg = null
        }
      }
      this._currentFileMsg = null
      this._currentCmdMsg = this.pushMessage('command', '', {
        command, output: '', running: true
      })
      return this._currentCmdMsg
    },
    ensureFileMsg(title) {
      this._currentAiMsg = null
      this._currentCmdMsg = null
      this._currentFileMsg = this.pushMessage('file', '', {
        title, detail: '', running: true
      })
      return this._currentFileMsg
    },
    handleAiEvent(data) {
      if (!data || typeof data !== 'object') return
      // 只处理本 Tab 对应服务器的任务事件
      if (data.serverID && data.serverID !== this.serverId) return
      if (this.currentTaskId && data.taskID !== this.currentTaskId) return

      switch (data.type) {
        case 'start':
          this.currentTaskId = data.taskID
          // 显示任务实际使用的接口配置与模式，便于确认配置加载正确
          const parts = []
          if (data.endpoint || data.model) parts.push(`接口: ${data.endpoint || '-'} · 模型: ${data.model || '-'}`)
          if (data.mode) parts.push(data.mode === 'interactive' ? '模式: 终端交互' : '模式: 后台执行')
          if (parts.length) this.pushMessage('status', parts.join(' · '))
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
        case 'file-read': {
          this.statusText = ''
          this.ensureFileMsg(`读取 ${data.path}`)
          break
        }
        case 'file-edit': {
          this.statusText = ''
          const opText = { insert: '插入', replace: '替换', delete: '删除', insert_after_match: '按关键字插入', replace_match: '按关键字替换' }[data.op] || data.op
          this.ensureFileMsg(`编辑 ${data.path}（${opText}）`)
          break
        }
        case 'file-read-done': {
          const m = this._currentFileMsg
          if (m) {
            m.running = false
            m.ok = !!data.ok
            this._currentFileMsg = null
          }
          break
        }
        case 'file-edit-done': {
          const m = this._currentFileMsg
          if (m) {
            m.running = false
            m.ok = !!data.ok
            m.detail = data.message || ''
            this._currentFileMsg = null
          }
          break
        }
        case 'final': {
          // 按正常 AI 对话展示：清理协议行后与普通回复一致，不再有单独的"总结"
          const m = this._currentAiMsg
          if (m) {
            const before = stripDirectiveLines(m.text, true).trimEnd()
            const idx = this.messages.indexOf(m)
            if (before.trim()) {
              m.text = before
            } else if (idx >= 0) {
              this.messages.splice(idx, 1)
            }
            this._currentAiMsg = null
          }
          const summary = (data.summary || '').trim()
          if (summary) this.pushMessage('ai', summary)
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
          this.setTermLock(false)
          this.trimHistory()
          break
      }
    },
    // 拖拽分隔条：调整对话区/终端区宽度（20%~80%），结束持久化
    startSplitDrag() {
      const main = this.$refs.mainEl
      if (!main) return
      document.body.style.userSelect = 'none'
      document.body.style.cursor = 'col-resize'
      const onMove = (ev) => {
        const rect = main.getBoundingClientRect()
        if (!rect.width) return
        let pct = ((ev.clientX - rect.left) / rect.width) * 100
        this.chatPct = Math.round(Math.min(80, Math.max(20, pct)) * 10) / 10
      }
      const onUp = () => {
        window.removeEventListener('mousemove', onMove)
        window.removeEventListener('mouseup', onUp)
        document.body.style.userSelect = ''
        document.body.style.cursor = ''
        try { localStorage.setItem(SPLIT_KEY, String(this.chatPct)) } catch (e) { /* ignore */ }
      }
      window.addEventListener('mousemove', onMove)
      window.addEventListener('mouseup', onUp)
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
/* ===== 主题变量（深色 / 浅色双套，跟随设置实时切换） ===== */
.ai-page.theme-dark {
  --text-main: #f2f2f2;        /* 正文主文字：保证深底高对比 */
  --text-secondary: #c9d4e0;   /* 次要文字 */
  --text-muted: #9db0c4;       /* 弱化文字 */
  --panel-bg: rgba(15, 20, 28, 0.72);
  --panel-border: #34404e;
  --tag-user-bg: #164e63; --tag-user-fg: #7dd3fc;
  --tag-ai-bg: #14532d; --tag-ai-fg: #86efac;
  --tag-cmd-bg: #713f12; --tag-cmd-fg: #fde047;
  --tag-file-bg: #14532d; --tag-file-fg: #95de64;
  --tag-error-bg: #7f1d1d; --tag-error-fg: #fecaca;
  --user-text: #e0f7ff;
  --error-text: #ff9d9d;
  --code-bg: #1a1a1a; --code-border: #3a4552; --code-fg: #ffd591;
  --cmd-code-fg: #ffe58f;
  --output-bg: #0d1117; --output-border: #30363d; --output-fg: #dce5f0;
  --detail-fg: #a7d8a7;
  --file-icon: #95de64;
}
.ai-page.theme-light {
  --text-main: #262626;        /* 浅底深字 */
  --text-secondary: #4b5563;
  --text-muted: #6b7280;
  --panel-bg: #ffffff;
  --panel-border: #d9d9d9;
  --tag-user-bg: #e6f4ff; --tag-user-fg: #0958d9;
  --tag-ai-bg: #f0fdee; --tag-ai-fg: #389e0d;
  --tag-cmd-bg: #fff7e6; --tag-cmd-fg: #d48806;
  --tag-file-bg: #f0fdee; --tag-file-fg: #389e0d;
  --tag-error-bg: #fff1f0; --tag-error-fg: #cf1322;
  --user-text: #0c4a6e;
  --error-text: #cf1322;
  --code-bg: #f6f8fa; --code-border: #e0e0e0; --code-fg: #b35000;
  --cmd-code-fg: #ad6800;
  --output-bg: #f6f8fa; --output-border: #e0e0e0; --output-fg: #24292e;
  --detail-fg: #389e0d;
  --file-icon: #389e0d;
}

.ai-page {
  height: 100%; display: flex; flex-direction: column;
  padding: 12px 16px 12px; box-sizing: border-box; overflow: hidden;
}

/* 左右分栏：对话区与终端区（宽度可拖拽调整） */
.ai-main { flex: 1; min-height: 0; display: flex; gap: 0; }
.ai-chat {
  flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 10px;
  padding-right: 12px; box-sizing: border-box;
}
.ai-term {
  flex: 1; min-width: 0;
  border: 1px solid var(--panel-border); border-radius: 8px;
  overflow: hidden; background: var(--panel-bg);
}
.ai-term-placeholder {
  height: 100%; display: flex; flex-direction: column; align-items: center;
  justify-content: center; gap: 10px; color: var(--text-muted);
}

/* 可拖拽分隔条 */
.splitter {
  flex: 0 0 10px; cursor: col-resize; display: flex;
  align-items: center; justify-content: center; touch-action: none;
}
.splitter-grip {
  width: 3px; height: 44px; border-radius: 2px;
  background: var(--panel-border); transition: background 0.2s, height 0.2s;
}
.splitter:hover .splitter-grip,
.splitter:active .splitter-grip {
  background: #1677ff; height: 76px;
}

/* 中部显示区域 */
.ai-messages {
  flex: 1; min-height: 0; overflow-y: auto; border: 1px solid var(--panel-border);
  border-radius: 8px; padding: 14px 16px; background: var(--panel-bg);
  font-size: 13.5px;
}
.ai-empty { text-align: center; color: var(--text-muted); padding-top: 100px; line-height: 2.2; }

.ai-warn-bar {
  flex-shrink: 0; color: #faad14; font-size: 12.5px;
  padding: 4px 10px; border: 1px dashed #faad14; border-radius: 6px;
}

.ai-msg { margin-bottom: 14px; }
.ai-msg-tag {
  display: inline-block; font-size: 12px; padding: 1px 8px; border-radius: 3px;
  margin-bottom: 5px; font-weight: 600;
}
.ai-msg.user .ai-msg-tag { background: var(--tag-user-bg); color: var(--tag-user-fg); }
.ai-msg.ai .ai-msg-tag { background: var(--tag-ai-bg); color: var(--tag-ai-fg); }
.ai-msg.cmd .ai-msg-tag { background: var(--tag-cmd-bg); color: var(--tag-cmd-fg); }
.ai-msg.file .ai-msg-tag { background: var(--tag-file-bg); color: var(--tag-file-fg); }
.ai-msg.error .ai-msg-tag { background: var(--tag-error-bg); color: var(--tag-error-fg); }

.ai-msg-body {
  font-size: 13.5px; line-height: 1.75; white-space: pre-wrap; word-break: break-word;
  color: var(--text-main);
}
.ai-msg.user .ai-msg-body { color: var(--user-text); }
.ai-msg.error .ai-msg-body { color: var(--error-text); font-weight: 500; }

/* Markdown 渲染元素（换行已转为 <br>，关闭 pre-wrap 防止双倍行距） */
.ai-msg-body.md { white-space: normal; }
.ai-msg-body :deep(code) {
  background: var(--code-bg); color: var(--code-fg); padding: 1px 6px; border-radius: 4px;
  border: 1px solid var(--code-border);
  font-family: Consolas, Monaco, monospace; font-size: 12.5px; white-space: pre-wrap;
}
.ai-msg-body :deep(strong) { color: var(--text-main); font-weight: 700; }
.ai-msg-body :deep(.md-h) { display: inline-block; margin-top: 2px; font-size: 14.5px; }
.ai-msg-body :deep(.md-li) { display: inline-block; margin-left: 2px; color: var(--text-secondary); }
.ai-msg-body :deep(.md-ol) { color: var(--text-secondary); }
.ai-msg-body :deep(.md-quote) {
  display: inline-block; margin: 1px 0; padding-left: 8px;
  border-left: 3px solid var(--panel-border); color: var(--text-secondary);
}
.ai-msg-body :deep(.md-hr) {
  display: block; height: 1px; background: var(--panel-border); margin: 6px 0;
}
.ai-msg-body :deep(.md-pre) {
  display: block; margin: 6px 0; padding: 10px 12px;
  background: var(--output-bg); border: 1px solid var(--output-border); border-radius: 6px;
  color: var(--output-fg); font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  line-height: 1.6; white-space: pre-wrap; word-break: break-all;
  max-height: 300px; overflow-y: auto;
}

.cmd-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.cmd-line code {
  background: var(--code-bg); border: 1px solid var(--code-border); padding: 5px 10px; border-radius: 4px;
  color: var(--cmd-code-fg); font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  flex: 1; min-width: 0; white-space: pre-wrap; word-break: break-all;
}
.cmd-tag { margin: 0; }
.file-icon { color: var(--file-icon); }
.file-detail { margin-top: 6px; color: var(--detail-fg); font-size: 12.5px; }
.cmd-output {
  margin: 6px 0 0; padding: 10px 12px; background: var(--output-bg);
  border: 1px solid var(--output-border); border-radius: 4px;
  color: var(--output-fg); font-family: Consolas, Monaco, monospace; font-size: 12.5px;
  line-height: 1.6; max-height: 280px; overflow-y: auto;
  white-space: pre-wrap; word-break: break-all;
}

.ai-status { color: var(--text-muted); font-size: 12.5px; text-align: center; margin: 4px 0; }
.cursor { animation: blink 1s infinite; color: #1677ff; }
@keyframes blink { 50% { opacity: 0; } }

/* 一体化输入框：无边框文本域 + 底部浮动按钮排，与输入框融为一体 */
.ai-composer {
  flex-shrink: 0; border: 1px solid var(--panel-border); border-radius: 10px;
  background: var(--panel-bg); padding: 4px 10px 6px;
  transition: border-color 0.2s;
}
.ai-composer:focus-within { border-color: #1677ff; }
.ai-composer :deep(.ant-input) {
  border: none; box-shadow: none; background: transparent; color: var(--text-main);
  font-size: 13.5px; resize: none; padding: 6px 2px;
}
.ai-composer :deep(.ant-input:focus) { box-shadow: none; }
.composer-bar {
  display: flex; align-items: center; justify-content: space-between;
  gap: 8px; flex-wrap: wrap;
}
.composer-bar .left { display: flex; align-items: center; gap: 8px; min-width: 0; flex-wrap: wrap; }
.composer-bar .right { display: flex; align-items: center; gap: 8px; min-width: 0; }
.ghost-btn { color: var(--text-muted); }
.ai-status-hint { color: #1677ff; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 280px; }
.send-btn { width: 32px; height: 32px; font-size: 14px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; }
</style>
