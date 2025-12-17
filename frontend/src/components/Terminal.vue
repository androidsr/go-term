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
      sessionClosed: false // 添加标志避免重复关闭
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
    },

    // 处理右键菜单
    handleContextMenu(event) {
      event.preventDefault();
      
      // 创建自定义右键菜单
      const menu = document.createElement('div');
      menu.className = 'terminal-context-menu';
      menu.style.position = 'absolute';
      menu.style.left = event.pageX + 'px';
      menu.style.top = event.pageY + 'px';
      menu.style.zIndex = '1000';
      menu.style.backgroundColor = '#fff';
      menu.style.border = '1px solid #ccc';
      menu.style.boxShadow = '0 2px 10px rgba(0,0,0,0.2)';
      menu.style.padding = '5px 0';
      
      // 复制选项
      const copyItem = document.createElement('div');
      copyItem.textContent = '复制';
      copyItem.style.padding = '5px 15px';
      copyItem.style.cursor = 'pointer';
      copyItem.onmouseover = () => copyItem.style.backgroundColor = '#f0f0f0';
      copyItem.onmouseout = () => copyItem.style.backgroundColor = 'transparent';
      copyItem.onclick = () => {
        // 使用xterm的API复制选中文本
        this.terminal.getSelection() && navigator.clipboard.writeText(this.terminal.getSelection());
        document.body.removeChild(menu);
      };
      menu.appendChild(copyItem);
      
      // 粘贴选项
      const pasteItem = document.createElement('div');
      pasteItem.textContent = '粘贴';
      pasteItem.style.padding = '5px 15px';
      pasteItem.style.cursor = 'pointer';
      pasteItem.onmouseover = () => pasteItem.style.backgroundColor = '#f0f0f0';
      pasteItem.onmouseout = () => pasteItem.style.backgroundColor = 'transparent';
      pasteItem.onclick = async () => {
        try {
          const text = await navigator.clipboard.readText();
          if (text) {
            // 发送粘贴的文本到终端
            this.onData(text);
          }
        } catch (err) {
          console.error('粘贴失败:', err);
        }
        document.body.removeChild(menu);
      };
      menu.appendChild(pasteItem);
      
      // 添加到页面
      document.body.appendChild(menu);
      
      // 点击其他地方关闭菜单
      const closeMenu = (e) => {
        if (!menu.contains(e.target)) {
          document.body.removeChild(menu);
          document.removeEventListener('click', closeMenu);
        }
      };
      
      // 延迟添加事件监听器，避免立即触发
      setTimeout(() => {
        document.addEventListener('click', closeMenu);
      }, 100);
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
  position: relative;
  background: #1e1e1e;
}

.terminal-element {
  height: 100%;
  padding: 0 0 4px 4px;
}

/* 隐藏滚动条 */
.terminal-element ::-webkit-scrollbar {
  display: none;
}

.terminal-element {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

/* 右键菜单样式 */
.terminal-context-menu {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  font-size: 14px;
  min-width: 100px;
}

.terminal-context-menu div {
  padding: 8px 16px;
  cursor: pointer;
  user-select: none;
}

.terminal-context-menu div:hover {
  background-color: #e6f7ff;
}
</style>