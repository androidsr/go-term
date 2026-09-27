<template>
  <div class="file-manager-container">
    <a-layout class="file-layout">
      <a-layout>
        <a-layout-content class="content">
          <div class="file-header">
            <div class="path-navigation">
              <a-button @click="goToParentDirectory" :disabled="isRootDirectory">
                <ArrowUpOutlined />上级
              </a-button>
              <a-input-search v-model:value="pathInput" placeholder="输入目录路径" style="width: 300px; margin-left: 10px;"
                @search="navigateToPath" />
              <a-input-search v-model:value="searchKeyword" placeholder="在当前目录内搜索文件名" style="width: 240px; margin-left: 10px;"
                allow-clear />
            </div>
            <div class="file-actions">
              <a-button @click="selectAndUploadFile">
                <UploadOutlined />上传文件
              </a-button>
              <a-button @click="showCreateFolderModal">
                <FolderAddOutlined />新建文件夹
              </a-button>
              <a-button @click="refreshFileList">
                <ReloadOutlined />刷新
              </a-button>
            </div>
          </div>

          <a-table :dataSource="displayFileList" :columns="fileColumns" :pagination="false" rowKey="name" :loading="loading"
            :scroll="{ y: 'calc(100vh - 220px)' }" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'">
                <div class="file-name-cell">
                  <FolderOutlined v-if="record.type === 'dir'" />
                  <FileOutlined v-else />
                  <span class="file-name" v-if="record.type === 'dir'" @click="handleFileClick(record)">{{ record.name }}</span>
                  <span v-else>{{ record.name }}</span>
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'size'">
                {{ formatFileSize(record.size) }}
              </template>
              <template v-else-if="column.dataIndex === 'mtime'">
                {{ formatDate(record.mtime) }}
              </template>
              <template v-else-if="column.dataIndex === 'action'">
                <a-space>
                  <a-button v-if="record.type === 'file'" size="small" @click="downloadFile(record)" :loading="downloading === record.path">
                    下载
                  </a-button>
                  <a-button size="small" danger @click="deleteFile(record)">删除</a-button>
                </a-space>
              </template>
            </template>
          </a-table>
        </a-layout-content>
      </a-layout>
    </a-layout>

    <!-- 新建文件夹模态框 -->
    <a-modal v-model:open="createFolderModalVisible" title="新建文件夹" @ok="handleCreateFolder">
      <a-form :model="folderForm" layout="vertical">
        <a-form-item label="文件夹名称" required>
          <a-input v-model:value="folderForm.name" placeholder="请输入文件夹名称" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 文件选择对话框 -->
    <input ref="fileInput" type="file" style="display: none" @change="onFileSelected" />
  </div>
</template>

<script>
import {
  FileOutlined,
  FolderOutlined,
  FolderAddOutlined,
  UploadOutlined,
  ReloadOutlined,
  ArrowUpOutlined
} from '@ant-design/icons-vue'
import {
  CreateSFTPClient,
  ListDirectory,
  UploadFileWithProgress,
  DownloadFileWithProgress,
  CreateDirectory,
  DeleteFile,
  GetLastPath,
  SetLastPath
} from '../../bindings/go-term/controllers/sshcontroller'
import { OpenFileDialog, SaveFileDialog } from '../../bindings/go-term/app'
import { Events } from '@wailsio/runtime'
import { taskStore, newUID } from '../store/taskStore.js'

export default {
  name: 'FileManager',
  components: {
    FileOutlined,
    FolderOutlined,
    FolderAddOutlined,
    UploadOutlined,
    ReloadOutlined,
    ArrowUpOutlined
  },
  props: {
    server: { type: Object, required: true },
    serverId: { type: String, required: true }
  },
  data() {
    return {
      loading: false,
      currentPath: '/',
      pathInput: '/',
      fileList: [],
      searchKeyword: '',
      createFolderModalVisible: false,
      folderForm: { name: '' },
      fileColumns: [
        { title: '名称', dataIndex: 'name', key: 'name' },
        { title: '大小', dataIndex: 'size', key: 'size' },
        { title: '修改时间', dataIndex: 'mtime', key: 'mtime' },
        { title: '操作', dataIndex: 'action', key: 'action' }
      ],
      downloading: '',
      lastPathLoaded: false
    }
  },
  computed: {
    isRootDirectory() {
      return this.currentPath === '/' || this.currentPath === ''
    },
    displayFileList() {
      const kw = (this.searchKeyword || '').trim().toLowerCase()
      if (!kw) return this.fileList
      return this.fileList.filter((f) => (f.name || '').toLowerCase().includes(kw))
    }
  },
  async mounted() {
    await this.initializeSFTP()
    // 恢复上次打开的目录
    try {
      const last = await GetLastPath(this.serverId)
      await this.loadFileList(last || '/')
    } catch (e) {
      await this.loadFileList('/')
    }
    this.setupProgressListeners()
  },
  beforeUnmount() {
    Events.Off('file-upload-progress')
    Events.Off('file-download-progress')
  },
  methods: {
    setupProgressListeners() {
      Events.On('file-upload-progress', (event) => {
        const data = event.data
        if (!data || !data.taskID) return
        taskStore.updateTransfer(data.taskID, {
          percent: Math.floor(data.percent),
          transferred: data.transferred,
          total: data.total,
          speed: this.computeSpeed(data)
        })
      })
      Events.On('file-download-progress', (event) => {
        const data = event.data
        if (!data || !data.taskID) return
        taskStore.updateTransfer(data.taskID, {
          percent: Math.floor(data.percent),
          transferred: data.transferred,
          total: data.total,
          speed: this.computeSpeed(data)
        })
      })
    },

    computeSpeed(data) {
      // 后端未直接给速度，这里用已传输量粗略估算（后端每次节流回调间隔约 100KB）
      return this.formatFileSize(data.transferred) + ' / ' + this.formatFileSize(data.total)
    },

    async initializeSFTP() {
      try {
        const result = await CreateSFTPClient(this.serverId)
        console.log('SFTP客户端创建结果:', result)
      } catch (error) {
        console.error('创建SFTP客户端失败:', error)
        this.$message.error(`创建SFTP客户端失败: ${error.message}`)
      }
    },

    async loadFileList(path = this.currentPath) {
      this.loading = true
      try {
        const files = await ListDirectory(this.serverId, path)
        this.currentPath = path
        this.pathInput = path
        this.fileList = files || []
        // 记录上次打开的目录
        SetLastPath(this.serverId, path).catch(() => {})
        if (!files || files.length === 0) {
          console.log('No files found in directory')
        }
      } catch (error) {
        console.error('加载文件列表失败:', error)
        this.$message.error(`加载文件列表失败: ${error.message}`)
        this.fileList = []
      } finally {
        this.loading = false
      }
    },

    refreshFileList() {
      this.loadFileList(this.currentPath)
    },

    handleFileClick(file) {
      if (file.type === 'dir') {
        this.loadFileList(file.path)
      }
    },

    async goToParentDirectory() {
      if (this.isRootDirectory) return
      const parentPath = this.currentPath.substring(0, this.currentPath.lastIndexOf('/'))
      if (parentPath === '') {
        await this.loadFileList('/')
      } else {
        await this.loadFileList(parentPath)
      }
    },

    async navigateToPath(path) {
      if (!path) return
      if (!path.startsWith('/')) path = '/' + path
      await this.loadFileList(path)
    },

    selectAndUploadFile() {
      this.selectFileToUpload()
    },

    async selectFileToUpload() {
      try {
        const localPath = await OpenFileDialog('选择要上传的文件', [
          { displayName: 'All Files', pattern: '*' }
        ])
        if (localPath) {
          const fileName = localPath.split('\\').pop().split('/').pop()
          const remotePath = `${this.currentPath}/${fileName}`
          this.startUpload(localPath, remotePath, fileName)
        }
      } catch (error) {
        console.error('上传文件失败:', error)
        this.$message.error(`上传文件失败: ${error.message}`)
      }
    },

    startUpload(localPath, remotePath, fileName) {
      const taskID = newUID('up')
      taskStore.addTransfer({
        id: taskID,
        type: 'upload',
        serverID: this.serverId,
        serverName: this.server ? this.server.name : this.serverId,
        name: fileName,
        status: 'running',
        total: 0,
        transferred: 0
      })
      taskStore.openDrawer('transfer')
      UploadFileWithProgress(this.serverId, taskID, localPath, remotePath)
        .then(() => {
          taskStore.updateTransfer(taskID, { percent: 100, status: 'success' })
          this.$message.success(`上传完成: ${fileName}`)
          this.refreshFileList()
        })
        .catch((err) => {
          taskStore.updateTransfer(taskID, { status: 'error', error: err.message })
          this.$message.error(`上传失败: ${err.message}`)
        })
    },

    onFileSelected(event) {
      const file = event.target.files[0]
      if (file) {
        event.target.value = ''
      }
    },

    async downloadFile(file) {
      try {
        const localPath = await SaveFileDialog('选择保存位置', file.name)
        if (localPath) {
          this.downloading = file.path
          const taskID = newUID('dl')
          taskStore.addTransfer({
            id: taskID,
            type: 'download',
            serverID: this.serverId,
            serverName: this.server ? this.server.name : this.serverId,
            name: file.name,
            status: 'running',
            total: file.size || 0,
            transferred: 0
          })
          taskStore.openDrawer('transfer')
          DownloadFileWithProgress(this.serverId, taskID, file.path, localPath)
            .then(() => {
              taskStore.updateTransfer(taskID, { percent: 100, status: 'success' })
              this.$message.success(`下载完成: ${file.name}`)
            })
            .catch((err) => {
              taskStore.updateTransfer(taskID, { status: 'error', error: err.message })
              this.$message.error(`下载失败: ${err.message}`)
            })
            .finally(() => { this.downloading = '' })
        }
      } catch (error) {
        console.error('下载文件失败:', error)
        this.$message.error(`下载文件失败: ${error.message}`)
        this.downloading = ''
      }
    },

    showCreateFolderModal() {
      this.folderForm.name = ''
      this.createFolderModalVisible = true
    },

    async handleCreateFolder() {
      if (!this.folderForm.name.trim()) {
        this.$message.warning('请输入文件夹名称')
        return
      }
      try {
        const folderPath = `${this.currentPath}/${this.folderForm.name}`
        const result = await CreateDirectory(this.serverId, folderPath)
        this.$message.success(result)
        this.createFolderModalVisible = false
        await this.loadFileList(this.currentPath)
      } catch (error) {
        console.error('创建文件夹失败:', error)
        this.$message.error(`创建文件夹失败: ${error.message}`)
      }
    },

    async deleteFile(file) {
      try {
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除',
            content: `确定要删除 ${file.name} 吗？`,
            okText: '确认', cancelText: '取消',
            onOk: () => resolve(), onCancel: () => reject('cancel')
          })
        })
        const result = await DeleteFile(this.serverId, file.path)
        this.$message.success(result)
        await this.loadFileList(this.currentPath)
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除文件失败:', error)
          this.$message.error(`删除文件失败: ${error.message}`)
        }
      }
    },

    formatFileSize(size) {
      if (!size) return '0 Bytes'
      const k = 1024
      const sizes = ['Bytes', 'KB', 'MB', 'GB']
      const i = Math.floor(Math.log(size) / Math.log(k))
      return parseFloat((size / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
    },

    formatDate(timestamp) {
      if (!timestamp) return '-'
      const date = new Date(timestamp * 1000)
      return date.toLocaleString('zh-CN')
    }
  }
}
</script>

<style scoped>
.file-manager-container { height: 100%; }
.file-layout { height: 100%; }
.content { padding: 16px; }
.file-header {
  display: flex; justify-content: space-between; align-items: center;
  margin-bottom: 16px; flex-wrap: wrap; gap: 10px;
}
.path-navigation { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.file-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.file-name-cell { display: flex; align-items: center; gap: 8px; }
.file-name { cursor: pointer; color: #1890ff; }
.file-name:hover { text-decoration: underline; }
@media (max-width: 768px) {
  .file-header { flex-direction: column; align-items: stretch; }
  .path-navigation { justify-content: center; }
  .file-actions { justify-content: center; }
}
</style>
