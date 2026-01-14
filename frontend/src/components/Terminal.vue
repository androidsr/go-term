<template>
  <div class="terminal-container">
    <div ref="terminalElement" class="terminal-element" @contextmenu="handleContextMenu"></div>

    <!-- 自定义右键菜单 -->
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
  </div>
</template>

<script>
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import '@xterm/xterm/css/xterm.css'
import {
  CreateTerminalSessionWithSize,
  ExecuteCommandWithoutNewline,
  InterruptCommand,
  ResizeTerminal,
  CloseTerminalSession,
  HandleFileUploadRequest,  // 添加导入
  HandleFileDownloadRequest  // 添加导入
} from '../../wailsjs/go/controllers/SSHController'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
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
    serverId: String
  },

  data() {
    return {
      terminal: null,
      fitAddon: null,
      sessionClosed: false, // 添加标志避免重复关闭
      writeTimer: null,
      writeBuffer: [],
      contextMenuVisible: false,
      contextMenuStyle: {
        position: 'absolute',
        display: 'none'
      }
    }
  },

  async mounted() {
    await this.initTerminal()
    window.addEventListener('resize', this.onResize)
    this.setupOutputListener()

    // 通知父组件终端已准备就绪
    this.$emit('terminal-ready', this.serverId);

    // 添加对发送命令事件的监听
    window.addEventListener('send-command-to-terminal', this.handleSendCommand);

    // 添加对文件上传/下载请求的监听
    window.addEventListener('file-upload-request', this.handleFileUpload);
    window.addEventListener('file-download-request', this.handleFileDownload);
  },

  beforeUnmount() {
    window.removeEventListener('resize', this.onResize)

    // 移除事件监听
    window.removeEventListener('send-command-to-terminal', this.handleSendCommand);
    window.removeEventListener('file-upload-request', this.handleFileUpload);
    window.removeEventListener('file-download-request', this.handleFileDownload);

    // 移除终端输出事件监听
    EventsOff(`terminal-output:${this.serverId}`);

    // 清理写入定时器
    if (this.writeTimer) {
      clearTimeout(this.writeTimer)
      this.writeTimer = null
    }

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
          cursorBlink: false,  // 关闭光标闪烁,减少重绘
          cursorStyle: 'block',  // 使用块状光标,更清晰
          cursorWidth: 1,
          theme: {
            background: '#1e1e1e',
            foreground: '#ffffff',
            selection: 'rgba(65, 105, 225, 0.3)',  // 蓝色半透明背景
            selectionForeground: '#ffffff'  // 选中文字保持白色
          },
          fontSize: 14,
          fontFamily: 'Consolas, Monaco, "Courier New", monospace',
          // 性能优化配置
          bufferSize: 1000,  // 降低到合理范围,减少DOM节点数量
          scrollback: 1000,
          allowProposedApi: true,
          fastScrollModifier: 'alt',
          fastScrollSensitivity: 5,
          rendererType: 'canvas',  // 改回Canvas,光标响应更快
          scrollSensitivity: 1,
          convertEol: true,
          bellStyle: 'none',  // 禁用铃声,减少干扰
          rightClickSelectsWord: false  // 禁用右键选词
        })

        this.fitAddon = new FitAddon()
        this.terminal.loadAddon(this.fitAddon)

        // 添加剪贴板支持
        const clipboardAddon = new ClipboardAddon()
        this.terminal.loadAddon(clipboardAddon)

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

      // 处理 Ctrl+L 清屏 - 直接清空当前终端显示
      if (ev.ctrlKey && ev.key === 'l') {
        ev.preventDefault()
        this.terminal.clear() // 清空终端显示
        return
      }

      // 处理 Ctrl+C
      if (ev.ctrlKey && ev.key === 'c') {
        ev.preventDefault()
        await ExecuteCommandWithoutNewline(this.serverId, '\x03') // Ctrl+C
        return
      }

      // 处理 Ctrl+V (粘贴)
      if (ev.ctrlKey && ev.key === 'v' && ev.shiftKey) {
        ev.preventDefault()
        // xterm.js会自动处理粘贴操作
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

    setupOutputListener() {
      const WRITE_DELAY = 8  // 批量写入延迟8ms

      const flushWriteBuffer = () => {
        if (this.writeBuffer.length > 0 && this.terminal) {
          // 批量写入所有累积的输出
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

      // 监听后端推送的输出事件
      EventsOn(`terminal-output:${this.serverId}`, (output) => {
        if (this.terminal && output) {
          this.writeBuffer.push(output)
          scheduleWrite()
        }
      })
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
    },

    /* ========== 右键菜单 ========== */

    // 处理右键菜单 - 使用 Ant Design Vue
    handleContextMenu(event) {
      event.preventDefault();

      // 设置菜单位置 - 使用 clientX/Y 配合 fixed 定位
      this.contextMenuStyle = {
        left: event.clientX + 'px',
        top: event.clientY + 'px',
        display: 'block'
      };

      // 显示菜单
      this.$nextTick(() => {
        this.contextMenuVisible = true;

        // 点击其他地方关闭菜单
        setTimeout(() => {
          document.addEventListener('click', this.closeContextMenu, { once: true });
        }, 100);
      });
    },

    // 处理菜单点击
    handleMenuClick({ key }) {
      this.contextMenuVisible = false;

      switch (key) {
        case 'copy':
          this.handleCopy();
          break;
        case 'paste':
          this.handlePaste();
          break;
        case 'interrupt':
          this.handleInterrupt();
          break;
        case 'clear':
          this.terminal.clear();
          break;
      }
    },

    // 关闭菜单方法
    closeContextMenu() {
      this.contextMenuVisible = false;
    },

    // 复制方法
    handleCopy() {
      const selection = this.terminal.getSelection();
      if (selection) {
        navigator.clipboard.writeText(selection).catch(err => {
          console.error('复制失败:', err);
        });
      }
      this.$nextTick(() => {
        this.terminal.focus();
      });
    },

    // 粘贴方法
    async handlePaste() {
      try {
        const text = await navigator.clipboard.readText();
        if (text) {
          this.onData(text);
          this.$nextTick(() => {
            this.terminal.focus();
          });
        }
      } catch (err) {
        console.error('粘贴失败:', err);
      }
    },

    // 中断命令方法 - 发送Ctrl+C
    async handleInterrupt() {
      try {
        // 使用专门的中断方法，确保在高输出场景下也能立即中断
        await InterruptCommand(this.serverId);
        this.$nextTick(() => {
          this.terminal.focus();
        });
      } catch (err) {
        console.error('中断命令失败:', err);
      }
    },

    // 处理发送命令事件
    handleSendCommand(event) {
      const { serverId, command } = event.detail;
      if (serverId === this.serverId) {
        this.sendCommand(command);
      }
    },

    // 添加发送命令的方法
    sendCommand(command) {
      if (this.terminal && typeof this.onData === 'function') {
        // 发送命令文本
        this.onData(command);
        // 发送回车键
        this.onData('\r');
      }
    },

    // 处理文件上传请求
    async handleFileUpload(event) {
      const { serverId, localPath, remotePath } = event.detail;
      if (serverId === this.serverId) {
        // 调用后端的文件上传方法
        try {
          await HandleFileUploadRequest(serverId, localPath, remotePath);
          console.log(`文件上传完成: ${localPath} -> ${remotePath}`);
          // 通过事件发送成功信息
          this.$emit('file-operation-success', {
            type: 'upload',
            localPath,
            remotePath
          });
        } catch (error) {
          console.error(`文件上传失败: ${error.message}`);
          // 通过事件发送错误信息
          this.$emit('file-operation-error', {
            type: 'upload',
            localPath,
            remotePath,
            error: error.message
          });
        }
      }
    },

    // 处理文件下载请求
    async handleFileDownload(event) {
      const { serverId, remotePath, localPath } = event.detail;
      if (serverId === this.serverId) {
        // 调用后端的文件下载方法
        try {
          await HandleFileDownloadRequest(serverId, remotePath, localPath);
          console.log(`文件下载完成: ${remotePath} -> ${localPath}`);
          // 通过事件发送成功信息
          this.$emit('file-operation-success', {
            type: 'download',
            remotePath,
            localPath
          });
        } catch (error) {
          console.error(`文件下载失败: ${error.message}`);
          // 通过事件发送错误信息
          this.$emit('file-operation-error', {
            type: 'download',
            remotePath,
            localPath,
            error: error.message
          });
        }
      }
    },

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
}

.terminal-element {
  flex: 1;
  padding: 0 0 0 4px;
  margin: 0;
  overflow: hidden;
}

/* 重写 xterm.js 默认样式以修复布局问题 */
.terminal-element :deep(.xterm) {
  height: 100% !important;
  width: 100% !important;
  background-color: #1e1e1e !important;
  border-radius: 0 !important;
  margin: 0 !important;
  /* 优化Canvas渲染性能 */
  image-rendering: pixelated;
}

.terminal-element :deep(.xterm-viewport) {
  background-color: #1e1e1e !important;
  scrollbar-color: #666 #1e1e1e;
  /* 启用GPU加速 */
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

/* 选中文字样式优化 */
.terminal-element :deep(.xterm-selection) {
  background: rgba(65, 105, 225, 0.4) !important;
  color: #ffffff !important;
}

/* Canvas层优化 */
.terminal-element :deep(.xterm-text-layer) {
  /* 确保文字层清晰 */
  text-rendering: optimizeLegibility;
}

.terminal-element :deep(.xterm-cursor-layer) {
  /* 光标层优先级 */
  z-index: 10;
}

/* 滚动条样式优化 */
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

/* 自定义右键菜单样式 */
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

</style>