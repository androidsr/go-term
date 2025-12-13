<template>
  <div class="terminal-container">
    <div ref="terminalElement" class="terminal-element"></div>
  </div>
</template>

<script>
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import {
  CreateTerminalSessionWithSize,
  ExecuteCommandWithoutNewline,
  ReadTerminalOutput,
  ResizeTerminal,
  CloseTerminalSession
} from '../../wailsjs/go/controllers/SSHController'

export default {
  name: 'TerminalComponent',
  props: {
    server: Object,
    serverId: String
  },

  data() {
    return {
      terminal: null,
      fitAddon: null,
      outputTimer: null,
      sessionClosed: false // 添加标志避免重复关闭
    }
  },

  async mounted() {
    await this.initTerminal()
    window.addEventListener('resize', this.onResize)
    this.startReadOutput()
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.onResize)
    clearInterval(this.outputTimer)
    
    // 清理终端实例，但只有在已加载且未被dispose的情况下才销毁
    if (this.terminal && typeof this.terminal.dispose === 'function') {
      try {
        this.terminal.dispose()
      } catch (e) {
        console.warn('Terminal dispose error (ignored):', e)
      }
      this.terminal = null
    }
    
    // 清理fitAddon，但只有在已加载的情况下才销毁
    if (this.fitAddon) {
      // fitAddon通常不需要显式销毁，但如果我们需要确保它被清理，可以设置为null
      this.fitAddon = null
    }
    
    // 如果组件销毁时仍有未完成的操作，尝试通知后端
    // 但只在会话尚未关闭且serverId存在的情况下调用
    if (this.serverId && !this.sessionClosed) {
      this.sessionClosed = true // 设置标志避免重复调用
      // 异步调用，不等待结果，避免阻塞销毁过程
      CloseTerminalSession(this.serverId).catch(err => {
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
          theme: { background: '#1e1e1e' },
          fontSize: 14,
          fontFamily: 'Consolas, Monaco, "Courier New", monospace'
        })

        this.fitAddon = new FitAddon()
        this.terminal.loadAddon(this.fitAddon)
        this.terminal.open(this.$refs.terminalElement)
        this.fitAddon.fit()

        // 直接将所有输入发送到后端，启用真正的PTY模式
        this.terminal.onData(this.onData)
        
        // 处理特殊按键
        this.terminal.onKey(this.onKey)

        // 获取当前窗口尺寸并创建终端会话
        const dims = this.fitAddon.proposeDimensions()
        let width = 80
        let height = 24
        if (dims && dims.cols > 0 && dims.rows > 0) {
          width = dims.cols
          height = dims.rows
        }

        await CreateTerminalSessionWithSize(this.serverId, width, height)

        this.$nextTick(() => {
          this.terminal.focus()
        })
      } catch (error) {
        console.error('初始化终端失败:', error)
        // 如果初始化失败，确保清理资源
        if (this.terminal && typeof this.terminal.dispose === 'function') {
          try {
            this.terminal.dispose()
          } catch (e) {
            console.warn('Terminal dispose error (ignored):', e)
          }
          this.terminal = null
        }
        if (this.fitAddon) {
          this.fitAddon = null
        }
        // 通知用户初始化失败
        throw new Error(`终端初始化失败: ${error.message}`)
      }
    },

    /* ========== 数据处理 ========== */

    // onData 处理所有输入数据，直接发送到后端
    onData: async function (data) {
      // 直接将所有输入数据发送到后端，启用真正的PTY模式
      await ExecuteCommandWithoutNewline(this.serverId, data)
    },

    // onKey 处理特殊按键组合
    onKey: async function (e) {
      const ev = e.domEvent
      
      // 处理 Ctrl+L 清屏
      if (ev.ctrlKey && ev.key === 'l') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.serverId, '\x0c') // Ctrl+L
        return
      }
      
      // 处理 Ctrl+C
      if (ev.ctrlKey && ev.key === 'c') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.serverId, '\x03') // Ctrl+C
        return
      }
      
      // 处理 Ctrl+Z
      if (ev.ctrlKey && ev.key === 'z') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.serverId, '\x1a') // Ctrl+Z
        return
      }
      
      // 处理 Ctrl+R (反向搜索历史)
      if (ev.ctrlKey && ev.key === 'r') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.serverId, '\x12') // Ctrl+R
        return
      }
    },

    /* ========== 输出读取 ========== */

    startReadOutput() {
      this.outputTimer = setInterval(async () => {
        const out = await ReadTerminalOutput(this.serverId)
        if (out) {
          this.terminal.write(out)
        }
      }, 50) // 更高的刷新频率以获得更流畅的体验
    },

    onResize() {
      this.fitAddon?.fit()
      
      // 同步窗口大小到远端TTY
      if (this.terminal && this.fitAddon) {
        const dims = this.fitAddon.proposeDimensions()
        if (dims && dims.cols > 0 && dims.rows > 0) {
          // 调用后端调整终端大小的功能
          ResizeTerminal(this.serverId, dims.cols, dims.rows)
            .catch(err => {
              console.error('调整终端大小失败:', err)
            })
        }
      }
    }
  }
}
</script>

<style scoped>
.terminal-container {
  height: 92vh;
  position: relative;
  background: #1e1e1e;
}

.terminal-element {
  height: 100%;
  padding: 10px;
}
</style>