<template>
  <div class="terminal-container">
    <div ref="terminalElement" class="terminal-element" @contextmenu="handleContextMenu"></div>
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
  ReadTerminalOutput,
  ResizeTerminal,
  CloseTerminalSession,
  HandleFileUploadRequest,  // 添加导入
  HandleFileDownloadRequest  // 添加导入
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
      sessionClosed: false, // 添加标志避免重复关闭
      consecutiveEmpty: 0,   // 连续空输出计数
      currentInterval: 50,  // 当前轮询间隔
      lastBufferCheck: 0,   // 上次缓冲区检查时间
      contextMenu: null     // 缓存右键菜单实例
    }
  },

  async mounted() {
    await this.initTerminal()
    window.addEventListener('resize', this.onResize)
    this.startReadOutput()

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
    clearInterval(this.outputTimer)

    // 移除事件监听
    window.removeEventListener('send-command-to-terminal', this.handleSendCommand);
    window.removeEventListener('file-upload-request', this.handleFileUpload);
    window.removeEventListener('file-download-request', this.handleFileDownload);

    // 清理右键菜单
    if (this.contextMenu) {
      document.body.removeChild(this.contextMenu);
      this.contextMenu = null;
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
          cursorBlink: true,
          theme: { background: '#1e1e1e' },
          fontSize: 14,
          fontFamily: 'Consolas, Monaco, "Courier New", monospace',
          // 限制缓冲区大小，防止 tail -f 等命令导致页面卡死
          bufferSize: 500,  // 最大保存500行历史记录，减少内存占用
          scrollback: 500    // 滚动缓冲区也设为500行
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

    startReadOutput() {
      const adjustPolling = () => {
        // 动态调整轮询间隔
        if (this.consecutiveEmpty > 20) { // 1秒无输出
          this.currentInterval = Math.min(200, this.currentInterval * 1.2) // 逐渐增加到最大200ms
        } else if (this.consecutiveEmpty === 0) { // 有新输出
          this.currentInterval = 50 // 恢复高频率
        }
      }

      const pollOutput = async () => {
        const out = await ReadTerminalOutput(this.serverId)
        if (out) {
          this.consecutiveEmpty = 0
          this.terminal.write(out)
          
          // 优化缓冲区检查 - 每秒检查一次而不是每次轮询
          const now = Date.now()
          if (now - this.lastBufferCheck > 1000) {
            this.lastBufferCheck = now
            const buffer = this.terminal.buffer.active
            if (buffer.length > 400) {
              this.terminal.scrollToBottom()
            }
          }
        } else {
          this.consecutiveEmpty++
        }
        
        adjustPolling()
        
        // 重新设置定时器以使用新间隔
        clearInterval(this.outputTimer)
        this.outputTimer = setInterval(pollOutput, this.currentInterval)
      }

      this.outputTimer = setInterval(pollOutput, this.currentInterval)
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

    // 处理右键菜单 - 优化版本，避免重复创建DOM
    handleContextMenu(event) {
      event.preventDefault();

      // 复用或创建菜单
      if (!this.contextMenu) {
        this.contextMenu = this.createContextMenu();
        document.body.appendChild(this.contextMenu);
      }

      // 更新位置
      this.contextMenu.style.left = event.pageX + 'px';
      this.contextMenu.style.top = event.pageY + 'px';
      this.contextMenu.style.display = 'block';

      // 点击其他地方关闭菜单
      setTimeout(() => {
        document.addEventListener('click', this.closeContextMenu, { once: true });
      }, 100);
    },

    // 创建右键菜单实例 - 只创建一次
    createContextMenu() {
      const menu = document.createElement('div');
      menu.className = 'terminal-context-menu';
      menu.style.position = 'absolute';
      menu.style.zIndex = '1000';
      menu.style.backgroundColor = '#fff';
      menu.style.border = '1px solid #ccc';
      menu.style.boxShadow = '0 2px 10px rgba(0,0,0,0.2)';
      menu.style.padding = '5px 0';
      menu.style.display = 'none';
      
      // 使用innerHTML和事件委托提高性能
      menu.innerHTML = `
        <div data-action="copy" class="menu-item">复制</div>
        <div data-action="paste" class="menu-item">粘贴</div>
      `;

      // 使用事件委托
      menu.addEventListener('click', (e) => {
        const action = e.target.dataset.action;
        if (action === 'copy') {
          this.handleCopy();
        } else if (action === 'paste') {
          this.handlePaste();
        }
        menu.style.display = 'none';
      });

      return menu;
    },

    // 关闭菜单方法
    closeContextMenu(e) {
      if (this.contextMenu && !this.contextMenu.contains(e.target)) {
        this.contextMenu.style.display = 'none';
      }
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
}

.terminal-element :deep(.xterm-viewport) {
  background-color: #1e1e1e !important;
  scrollbar-color: #666 #1e1e1e;
}

.terminal-element :deep(.xterm-screen) {
  background-color: #1e1e1e !important;
  padding: 0 !important;
  margin: 0 !important;
}

.terminal-element :deep(.xterm-helper-textarea) {
  background-color: #1e1e1e !important;
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

/* 右键菜单样式 */
.terminal-context-menu {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-size: 14px;
  min-width: 100px;
}

.terminal-context-menu .menu-item {
  padding: 8px 16px;
  cursor: pointer;
  user-select: none;
  transition: background-color 0.1s ease;
}

.terminal-context-menu .menu-item:hover {
  background-color: #e6f7ff;
}
</style>