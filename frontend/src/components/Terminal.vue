<template>
  <div class="terminal-container" @dragover.prevent="onDragOver" @dragleave="onDragLeave" @drop="handleDrop">
    <div ref="terminalElement" class="terminal-element" @contextmenu="handleContextMenu"></div>

    <!-- 终端内查找（基于缓冲区，无需额外依赖） -->
    <div v-if="findVisible" class="find-bar">
      <a-input
        ref="findInput"
        v-model:value="findText"
        size="small"
        placeholder="查找 (Enter 下一个, Shift+Enter 上一个)"
        style="width: 240px"
        @pressEnter="onFindEnter"
        @input="runFind"
      />
      <span class="find-count">{{ findMatches.length ? findCursor + 1 : 0 }}/{{ findMatches.length }}</span>
      <a-button size="small" @click="findNext">下一个</a-button>
      <a-button size="small" @click="findPrev">上一个</a-button>
      <a-button size="small" type="text" @click="closeFind">✕</a-button>
    </div>

    <!-- 自定义右键菜单（自动避开屏幕边缘） -->
    <div v-if="contextMenuVisible" class="custom-context-menu" :style="contextMenuStyle">
      <div class="menu-item" @click="handleMenuClick({ key: 'copy' })">
        <CopyOutlined /> 复制
      </div>
      <div class="menu-item" @click="handleMenuClick({ key: 'paste' })">
        <ScissorOutlined /> 粘贴
      </div>
      <div class="menu-divider"></div>
      <div class="menu-item danger-menu-item" @click="handleMenuClick({ key: 'interrupt' })">
        <StopOutlined /> 中断命令
      </div>
      <div class="menu-divider"></div>
      <div class="menu-item" @click="handleMenuClick({ key: 'find' })">
        <SearchOutlined /> 查找
      </div>
      <div class="menu-divider"></div>
      <div class="menu-item" @click="handleMenuClick({ key: 'clear' })">
        <ClearOutlined /> 清空屏幕
      </div>
    </div>

    <!-- 复制提示 / 拖拽提示 -->
    <div v-if="copyHint" class="copy-hint">已复制到剪贴板</div>
    <div v-if="dropActive" class="drop-hint">松开以上传到当前目录</div>
  </div>
</template>

<script>
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import '@xterm/xterm/css/xterm.css'
import {
  ResizeTerminal,
  CloseTerminalSession,
  InterruptCommand,
  ExecuteCommandWithoutNewline,
  GetLastPath,
  EnsureSFTPClient,
  UploadFileWithProgress
} from '../../bindings/go-term/controllers/sshcontroller'
import { Events } from '@wailsio/runtime'
import { taskStore, newUID } from '../store/taskStore.js'
import { settingsStore, xtermTheme } from '../store/settingsStore.js'
import {
  CopyOutlined,
  ScissorOutlined,
  StopOutlined,
  ClearOutlined,
  SearchOutlined
} from '@ant-design/icons-vue'

export default {
  name: 'TerminalComponent',
  components: {
    CopyOutlined,
    ScissorOutlined,
    StopOutlined,
    ClearOutlined,
    SearchOutlined
  },
  props: {
    server: Object,
    serverId: String,
    sessionId: { type: String, required: true },
    active: { type: Boolean, default: false }
  },

  data() {
    return {
      terminal: null,
      fitAddon: null,
      sessionClosed: false,
      writeTimer: null,
      writeBuffer: [],
      contextMenuVisible: false,
      contextMenuStyle: { left: '0px', top: '0px' },
      copyHint: false,
      copyHintTimer: null,
      lastHintAt: 0,
      // 查找
      findVisible: false,
      findText: '',
      findMatches: [],
      findCursor: 0,
      // 拖拽
      dropActive: false,
      remoteBase: '.',
      resizeObserver: null,
      fitRaf: null
    }
  },

  watch: {
    active(newVal) {
      if (newVal && this.terminal) {
        this.$nextTick(() => {
          this.fitAndResize()
          this.terminal.focus()
        })
      }
    }
  },

  async mounted() {
    await this.initTerminal()
    this.setupResizeObserver()
    window.addEventListener('resize', this.onResize)
    this.setupOutputListener()
    this.$emit('terminal-ready', this.sessionId)
    window.addEventListener('apply-theme', this.applyTerminalOptions)
    // 记录上传基准目录
    try {
      const p = await GetLastPath(this.serverId)
      if (p) this.remoteBase = p
    } catch (e) {
      /* ignore */
    }
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.onResize)
    window.removeEventListener('apply-theme', this.applyTerminalOptions)

    if (this.resizeObserver) {
      this.resizeObserver.disconnect()
      this.resizeObserver = null
    }
    if (this.fitRaf) {
      cancelAnimationFrame(this.fitRaf)
      this.fitRaf = null
    }

    Events.Off(`terminal-output:${this.sessionId}`)

    if (this.writeTimer) {
      clearTimeout(this.writeTimer)
      this.writeTimer = null
    }
    if (this.copyHintTimer) clearTimeout(this.copyHintTimer)

    if (this.terminal && typeof this.terminal.dispose === 'function') {
      try {
        this.terminal.dispose()
      } catch (e) {
        console.warn('Terminal dispose error (ignored):', e)
      }
      this.terminal = null
    }
    this.fitAddon = null

    if (this.sessionId && !this.sessionClosed) {
      this.sessionClosed = true
      CloseTerminalSession(this.sessionId).catch((err) => {
        console.warn('Failed to close terminal session on unmount:', err)
      })
    }
  },

  methods: {
    /* ========== 初始化 ========== */
    async initTerminal() {
      try {
        this.terminal = new Terminal({
          cursorBlink: true,
          cursorStyle: settingsStore.cursorStyle,
          cursorWidth: 1,
          theme: xtermTheme(),
          fontSize: settingsStore.fontSize,
          fontFamily: settingsStore.fontFamily,
          bufferSize: settingsStore.scrollback,
          scrollback: settingsStore.scrollback,
          allowProposedApi: true,
          fastScrollModifier: 'alt',
          fastScrollSensitivity: 5,
          rendererType: 'canvas',
          scrollSensitivity: 1,
          convertEol: true,
          bellStyle: 'none',
          rightClickSelectsWord: false
        })

        this.fitAddon = new FitAddon()
        this.terminal.loadAddon(this.fitAddon)
        this.terminal.loadAddon(new ClipboardAddon())

        this.terminal.open(this.$refs.terminalElement)
        this.fitAddon.fit()

        this.terminal.onData(this.onData)
        this.terminal.onKey(this.onKey)

        this.terminal.onSelectionChange(() => {
          const sel = this.terminal.getSelection()
          if (sel && sel.length > 0) {
            navigator.clipboard.writeText(sel).catch(() => {})
            this.flashCopyHint()
          }
        })

        const dims = this.fitAddon.proposeDimensions()
        let width = 80
        let height = 24
        if (dims && dims.cols > 0 && dims.rows > 0) {
          width = dims.cols
          height = dims.rows
        }

        ResizeTerminal(this.sessionId, width, height).catch((err) => {
          console.warn('调整终端大小失败:', err)
        })

        this.$nextTick(() => {
          if (this.active) this.terminal.focus()
        })
      } catch (error) {
        console.error('初始化终端失败:', error)
        throw new Error(`终端初始化失败: ${error.message}`)
      }
    },

    // 应用主题 / 字号等设置
    applyTerminalOptions() {
      if (!this.terminal) return
      try {
        this.terminal.options.theme = xtermTheme()
        this.terminal.options.fontSize = settingsStore.fontSize
        this.terminal.options.fontFamily = settingsStore.fontFamily
        this.terminal.options.cursorStyle = settingsStore.cursorStyle
        this.fitAddon && this.fitAddon.fit()
      } catch (e) {
        console.warn('应用终端设置失败:', e)
      }
    },

    /* ========== 数据处理 ========== */
    onData: async function (data) {
      await ExecuteCommandWithoutNewline(this.sessionId, data)
    },

    onKey: async function (e) {
      const ev = e.domEvent

      if (ev.ctrlKey && ev.key === 'l') {
        ev.preventDefault()
        this.terminal.clear()
        return
      }
      if (ev.ctrlKey && ev.key === 'c') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.sessionId, '\x03')
        return
      }
      if (ev.ctrlKey && ev.key === 'f') {
        ev.preventDefault()
        this.openFind()
        return
      }
      if (ev.ctrlKey && ev.key === 'v' && ev.shiftKey) {
        ev.preventDefault()
        return
      }
      if (ev.ctrlKey && ev.key === 'z') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.sessionId, '\x1a')
        return
      }
      if (ev.ctrlKey && ev.key === 'r') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.sessionId, '\x12')
        return
      }
    },

    /* ========== 输出读取 ========== */
    setupOutputListener() {
      const WRITE_DELAY = 8
      const flushWriteBuffer = () => {
        if (this.writeBuffer.length > 0 && this.terminal) {
          const combined = this.writeBuffer.join('')
          this.terminal.write(combined)
          this.writeBuffer = []
        }
        this.writeTimer = null
      }
      const scheduleWrite = () => {
        if (this.writeTimer || !this.terminal) return
        this.writeTimer = setTimeout(flushWriteBuffer, WRITE_DELAY)
      }

      Events.On(`terminal-output:${this.sessionId}`, (event) => {
        const output = event.data
        if (this.terminal && output) {
          this.writeBuffer.push(output)
          scheduleWrite()
        }
      })
    },

    // 按当前容器尺寸重算 cols/rows 并同步给后端 pty
    fitAndResize() {
      if (!this.fitAddon || !this.terminal) return
      try {
        this.fitAddon.fit()
      } catch (e) {
        return
      }
      const dims = this.fitAddon.proposeDimensions()
      if (dims && dims.cols > 0 && dims.rows > 0) {
        ResizeTerminal(this.sessionId, dims.cols, dims.rows).catch((err) => {
          console.warn('调整终端大小失败:', err)
        })
      }
    },

    // 监听容器自身尺寸变化：切换标签页、分屏、窗口最大化等都会触发
    setupResizeObserver() {
      if (typeof ResizeObserver === 'undefined') return
      const el = this.$refs.terminalElement
      if (!el) return
      this.resizeObserver = new ResizeObserver(() => {
        if (!this.terminal) return
        // 容器不可见（后台标签页 display:none）时尺寸为 0，跳过，等真正可见后再算
        if (el.clientWidth === 0 || el.clientHeight === 0) return
        if (this.fitRaf) cancelAnimationFrame(this.fitRaf)
        this.fitRaf = requestAnimationFrame(() => this.fitAndResize())
      })
      this.resizeObserver.observe(el)
    },

    onResize() {
      this.fitAndResize()
    },

    /* ========== 右键菜单 ========== */
    handleContextMenu(event) {
      event.preventDefault()
      const menuW = 180
      const menuH = 240
      const margin = 8
      const vw = window.innerWidth
      const vh = window.innerHeight
      let left = event.clientX
      let top = event.clientY
      if (left + menuW > vw - margin) left = vw - menuW - margin
      if (top + menuH > vh - margin) top = vh - menuH - margin
      if (left < margin) left = margin
      if (top < margin) top = margin
      this.contextMenuStyle = { left: left + 'px', top: top + 'px' }
      this.$nextTick(() => {
        this.contextMenuVisible = true
        setTimeout(() => {
          document.addEventListener('click', this.closeContextMenu, { once: true })
        }, 100)
      })
    },

    handleMenuClick({ key }) {
      this.contextMenuVisible = false
      switch (key) {
        case 'copy':
          this.handleCopy()
          break
        case 'paste':
          this.handlePaste()
          break
        case 'interrupt':
          this.handleInterrupt()
          break
        case 'find':
          this.openFind()
          break
        case 'clear':
          this.terminal.clear()
          break
      }
    },

    closeContextMenu() {
      this.contextMenuVisible = false
    },

    handleCopy() {
      const selection = this.terminal.getSelection()
      if (selection) {
        navigator.clipboard.writeText(selection).catch((err) => {
          console.error('复制失败:', err)
        })
      }
      this.$nextTick(() => this.terminal.focus())
    },

    async handlePaste() {
      try {
        const text = await navigator.clipboard.readText()
        if (text) {
          this.onData(text)
          this.$nextTick(() => this.terminal.focus())
        }
      } catch (err) {
        console.error('粘贴失败:', err)
      }
    },

    async handleInterrupt() {
      try {
        await InterruptCommand(this.sessionId)
        this.$nextTick(() => this.terminal.focus())
      } catch (err) {
        console.error('中断命令失败:', err)
      }
    },

    flashCopyHint() {
      const now = Date.now()
      if (now - this.lastHintAt < 1500) return
      this.lastHintAt = now
      this.copyHint = true
      if (this.copyHintTimer) clearTimeout(this.copyHintTimer)
      this.copyHintTimer = setTimeout(() => {
        this.copyHint = false
      }, 1200)
    },

    /* ========== 终端内查找（基于缓冲区） ========== */
    openFind() {
      this.findVisible = true
      this.$nextTick(() => {
        this.$refs.findInput && this.$refs.findInput.focus()
      })
    },
    closeFind() {
      this.findVisible = false
      this.findText = ''
      this.findMatches = []
      this.findCursor = 0
    },
    onFindEnter(e) {
      if (e && e.shiftKey) this.findPrev()
      else this.findNext()
    },
    runFind() {
      this.findMatches = []
      this.findCursor = 0
      if (!this.findText || !this.terminal) return
      const buffer = this.terminal.buffer.active
      for (let i = 0; i < buffer.length; i++) {
        const line = buffer.getLine(i)
        if (line && line.translateToString(true).includes(this.findText)) {
          this.findMatches.push(i)
        }
      }
      if (this.findMatches.length) {
        this.terminal.scrollToLine(this.findMatches[0])
      }
    },
    findNext() {
      if (!this.findMatches.length) return
      this.findCursor = (this.findCursor + 1) % this.findMatches.length
      this.terminal.scrollToLine(this.findMatches[this.findCursor])
    },
    findPrev() {
      if (!this.findMatches.length) return
      this.findCursor = (this.findCursor - 1 + this.findMatches.length) % this.findMatches.length
      this.terminal.scrollToLine(this.findMatches[this.findCursor])
    },

    /* ========== 拖拽上传 ========== */
    onDragOver() {
      this.dropActive = true
    },
    onDragLeave(e) {
      if (e.target === this.$el) this.dropActive = false
    },
    async handleDrop(e) {
      e.preventDefault()
      this.dropActive = false
      const files = e.dataTransfer && e.dataTransfer.files
      if (!files || !files.length) return
      for (let i = 0; i < files.length; i++) {
        this.uploadFile(files[i].path)
      }
    },
    async uploadFile(localPath) {
      if (!localPath) return
      const name = localPath.split(/[\\/]/).pop()
      const remotePath = this.remoteBase && this.remoteBase !== '.'
        ? this.remoteBase.replace(/\/$/, '') + '/' + name
        : name
      try {
        await EnsureSFTPClient(this.serverId)
        const taskID = newUID('up')
        taskStore.addTransfer({
          id: taskID,
          serverID: this.serverId,
          serverName: this.server ? this.server.name : this.serverId,
          localPath,
          remotePath,
          transferred: 0,
          total: 0,
          percent: 0,
          status: 'transferring'
        })
        taskStore.openDrawer('transfer')
        await UploadFileWithProgress(this.serverId, taskID, localPath, remotePath)
        taskStore.updateTransfer(taskID, { status: 'done', percent: 100 })
      } catch (err) {
        this.$message && this.$message.error('上传失败: ' + err.message)
        console.error('拖拽上传失败:', err)
      }
    }
  }
}
</script>

<style scoped>
.terminal-container {
  height: 100%;
  display: flex;
  flex-direction: column;
  margin: 0;
  background: #1e1e1e;
  overflow: hidden;
  position: relative;
}

.terminal-element {
  flex: 1;
  padding: 0 0 0 4px;
  margin: 0;
  overflow: hidden;
  min-height: 0;
}

.terminal-element :deep(.xterm) {
  height: 100% !important;
  width: 100% !important;
  background-color: #1e1e1e !important;
  border-radius: 0 !important;
  margin: 0 !important;
  image-rendering: pixelated;
}

.terminal-element :deep(.xterm-viewport) {
  background-color: #1e1e1e !important;
  scrollbar-color: #666 #1e1e1e;
  transform: translateZ(0);
  will-change: scroll-position;
}

.terminal-element :deep(.xterm-screen) {
  background-color: #1e1e1e !important;
  padding: 0 !important;
  margin: 0 !important;
}

.terminal-element :deep(.xterm-helper-textarea) {
  background-color: #1e1e1e !important;
}

.terminal-element :deep(.xterm-selection) {
  background: rgba(65, 105, 225, 0.4) !important;
  color: #ffffff !important;
}

.terminal-element :deep(.xterm-text-layer) {
  text-rendering: optimizeLegibility;
}

.terminal-element :deep(.xterm-cursor-layer) {
  z-index: 10;
}

.terminal-element :deep(.xterm-viewport::-webkit-scrollbar) {
  width: 8px;
}
.terminal-element :deep(.xterm-viewport::-webkit-scrollbar-track) {
  background: #1e1e1e;
}
.terminal-element :deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: #666;
  border-radius: 4px;
}
.terminal-element :deep(.xterm-viewport::-webkit-scrollbar-thumb:hover) {
  background: #888;
}

.find-bar {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 10002;
  display: flex;
  align-items: center;
  gap: 6px;
  background: #2d2d2d;
  padding: 6px 8px;
  border-radius: 4px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.35);
  border: 1px solid #444;
}
.find-count {
  color: #bbb;
  font-size: 12px;
  min-width: 40px;
  text-align: center;
}

.custom-context-menu {
  position: fixed;
  z-index: 10000;
  background: var(--antd-color-bg-container);
  border-radius: 4px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  min-width: 160px;
  padding: 4px 0;
  border: 1px solid var(--antd-color-border);
  margin: 0;
}

.menu-item {
  padding: 8px 16px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--antd-color-text);
  transition: background-color 0.3s;
}

.menu-item:hover {
  background-color: var(--antd-color-bg-text-hover);
}

.menu-divider {
  height: 1px;
  background-color: var(--antd-color-border);
  margin: 4px 0;
}

.danger-menu-item {
  color: var(--antd-color-error) !important;
}

.danger-menu-item:hover {
  background-color: rgba(255, 77, 79, 0.1) !important;
}

.copy-hint {
  position: absolute;
  bottom: 16px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(0, 0, 0, 0.75);
  color: #fff;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 12px;
  z-index: 10001;
  pointer-events: none;
}

.drop-hint {
  position: absolute;
  inset: 0;
  background: rgba(24, 144, 255, 0.12);
  border: 2px dashed #1890ff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  color: #1890ff;
  z-index: 10003;
  pointer-events: none;
}
</style>
