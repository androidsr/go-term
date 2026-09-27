<template>
  <div class="terminal-container">
    <div ref="terminalElement" class="terminal-element" @contextmenu="handleContextMenu"></div>

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
      <div class="menu-item" @click="handleMenuClick({ key: 'clear' })">
        <ClearOutlined /> 清空屏幕
      </div>
    </div>

    <!-- 复制提示（轻量，不阻塞） -->
    <div v-if="copyHint" class="copy-hint">已复制到剪贴板</div>
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
  ExecuteCommandWithoutNewline
} from '../../bindings/go-term/controllers/sshcontroller'
import { Events } from '@wailsio/runtime'
import { CopyOutlined, ScissorOutlined, StopOutlined, ClearOutlined } from '@ant-design/icons-vue'

export default {
  name: 'TerminalComponent',
  components: {
    CopyOutlined,
    ScissorOutlined,
    StopOutlined,
    ClearOutlined
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
      contextMenuStyle: {
        left: '0px',
        top: '0px'
      },
      copyHint: false,
      copyHintTimer: null,
      lastHintAt: 0
    }
  },

  watch: {
    active(newVal) {
      // 修复：切换标签页时光标锁定在选项卡标题、输出无效的问题
      if (newVal && this.terminal) {
        this.$nextTick(() => this.terminal.focus())
      }
    }
  },

  async mounted() {
    await this.initTerminal()
    window.addEventListener('resize', this.onResize)
    this.setupOutputListener()
    this.$emit('terminal-ready', this.sessionId)
    window.addEventListener('send-command-to-terminal', this.handleSendCommand)
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.onResize)
    window.removeEventListener('send-command-to-terminal', this.handleSendCommand)

    Events.Off(`terminal-output:${this.sessionId}`)

    if (this.writeTimer) {
      clearTimeout(this.writeTimer)
      this.writeTimer = null
    }

    if (this.copyHintTimer) {
      clearTimeout(this.copyHintTimer)
    }

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
          cursorStyle: 'block',
          cursorWidth: 1,
          theme: {
            background: '#1e1e1e',
            foreground: '#ffffff',
            selection: 'rgba(65, 105, 225, 0.3)',
            selectionForeground: '#ffffff'
          },
          fontSize: 14,
          fontFamily: 'Consolas, Monaco, "Courier New", monospace',
          bufferSize: 1000,
          scrollback: 1000,
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

        const clipboardAddon = new ClipboardAddon()
        this.terminal.loadAddon(clipboardAddon)

        this.terminal.open(this.$refs.terminalElement)
        this.fitAddon.fit()

        this.terminal.onData(this.onData)
        this.terminal.onKey(this.onKey)

        // 选中即复制到剪贴板
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

        // 通知后端调整尺寸（会话已在 ServerManager 中创建）
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

    onResize() {
      if (!this.fitAddon) return
      this.fitAddon.fit()
      if (this.terminal && this.fitAddon) {
        const dims = this.fitAddon.proposeDimensions()
        if (dims && dims.cols > 0 && dims.rows > 0) {
          ResizeTerminal(this.sessionId, dims.cols, dims.rows).catch((err) => {
            console.error('调整终端大小失败:', err)
          })
        }
      }
    },

    /* ========== 右键菜单（自动避开边缘） ========== */
    handleContextMenu(event) {
      event.preventDefault()

      const menuW = 180
      const menuH = 200
      const margin = 8
      const vw = window.innerWidth
      const vh = window.innerHeight

      let left = event.clientX
      let top = event.clientY

      if (left + menuW > vw - margin) left = vw - menuW - margin
      if (top + menuH > vh - margin) top = vh - menuH - margin
      if (left < margin) left = margin
      if (top < margin) top = margin

      this.contextMenuStyle = {
        left: left + 'px',
        top: top + 'px'
      }

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

    handleSendCommand(event) {
      const { serverId, command } = event.detail
      if (serverId === this.sessionId) {
        this.sendCommand(command)
      }
    },

    sendCommand(command) {
      if (this.terminal && typeof this.onData === 'function') {
        this.onData(command)
        this.onData('\r')
      }
    }
  }
}
</script>

<style scoped>
.terminal-container {
  height: calc(100vh - 52px);
  display: flex;
  margin: 0;
  flex-direction: column;
  background: #1e1e1e;
  overflow: hidden;
  position: relative;
}

.terminal-element {
  flex: 1;
  padding: 0 0 0 4px;
  margin: 0;
  overflow: hidden;
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
</style>
