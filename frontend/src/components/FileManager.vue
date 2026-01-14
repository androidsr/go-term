<template>
  <div class="file-manager-container">
    <a-layout class="file-layout">
      <!-- 右侧文件列表 -->
      <a-layout>
        <a-layout-content class="content">
          <div class="file-header">
            <div class="path-navigation">
              <a-button @click="goToParentDirectory" :disabled="isRootDirectory">
                <ArrowUpOutlined />上级
              </a-button>
              <a-input-search v-model:value="pathInput" placeholder="输入目录路径" style="width: 300px; margin-left: 10px;"
                @search="navigateToPath" />
            </div>
            <div class="file-actions">
              <a-button @click="selectAndUploadFile" :loading="uploading">
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

          <a-table :dataSource="fileList" :columns="fileColumns" :pagination="false" rowKey="name" :loading="loading"
            :scroll="{ y: 'calc(100vh - 200px)' }" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'">
                <div class="file-name-cell">
                  <FolderOutlined v-if="record.type === 'dir'" />
                  <FileOutlined v-else />
                  <span class="file-name" v-if="record.type === 'dir'" @click="handleFileClick(record)">{{ record.name
                    }}</span>
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
                  <a-button v-if="record.type === 'file'" size="small" @click="downloadFile(record)" :loading="downloading">
                    下载
                  </a-button>
                  <a-button size="small" @click="deleteFile(record)">删除</a-button>
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

    <!-- 进度条模态框 -->
    <a-modal v-model:open="progressModalVisible" :title="progressTitle" :closable="false" :maskClosable="false"
      :footer="null" width="500px">
      <div class="progress-container">
        <div class="progress-info">
          <span>{{ progressInfo.fileName }}</span>
          <span>{{ progressInfo.transferred }} / {{ progressInfo.total }}</span>
        </div>
        <a-progress :percent="progressPercent" :status="progressStatus" />
        <div class="progress-speed">
          <span>{{ progressInfo.speed }}</span>
        </div>
      </div>
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
} from '@ant-design/icons-vue';
import {
  CreateSFTPClient,
  ListDirectory,
  UploadFileWithProgress,
  DownloadFileWithProgress,
  CreateDirectory,
  DeleteFile
} from '../../wailsjs/go/controllers/SSHController';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

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
    server: {
      type: Object,
      required: true
    },
    serverId: {
      type: String,
      required: true
    }
  },
  data() {
    return {
      loading: false,
      currentPath: '/',
      pathInput: '/',
      fileList: [],
      createFolderModalVisible: false,
      folderForm: {
        name: ''
      },
      fileColumns: [
        {
          title: '名称',
          dataIndex: 'name',
          key: 'name'
        },
        {
          title: '大小',
          dataIndex: 'size',
          key: 'size'
        },
        {
          title: '修改时间',
          dataIndex: 'mtime',
          key: 'mtime'
        },
        {
          title: '操作',
          dataIndex: 'action',
          key: 'action'
        }
      ],
      // 进度条相关
      progressModalVisible: false,
      progressTitle: '',
      progressPercent: 0,
      progressStatus: 'active',
      progressInfo: {
        fileName: '',
        transferred: '0 B',
        total: '0 B',
        speed: '0 B/s'
      },
      // 用于追踪速度计算
      progressStartTime: null,
      lastSpeedUpdate: 0,
      uploading: false,
      downloading: false
    };
  },
  computed: {
    isRootDirectory() {
      return this.currentPath === '/' || this.currentPath === '';
    }
  },
  async mounted() {
    await this.initializeSFTP();
    await this.loadFileList();
    // 监听进度事件
    this.setupProgressListeners();
  },
  beforeUnmount() {
    // 清理进度事件监听
    EventsOff('file-upload-progress');
    EventsOff('file-download-progress');
    if (this.progressTimer) {
      clearInterval(this.progressTimer);
    }
  },
  methods: {
    setupProgressListeners() {
      // 监听上传进度
      EventsOn('file-upload-progress', (data) => {
        const percent = Math.floor(data.percent);
        this.progressPercent = percent;
        this.progressInfo.transferred = this.formatFileSize(data.transferred);
        // 只在第一次更新 total，避免重复计算
        if (!this.progressInfo.total || data.total > 0) {
          this.progressInfo.total = this.formatFileSize(data.total);
        }

        // 节流速度计算，每 500ms 更新一次
        const now = Date.now();
        if (this.progressStartTime && (now - this.lastSpeedUpdate > 500 || data.percent === 100)) {
          const elapsed = (now - this.progressStartTime) / 1000;
          if (elapsed > 0) {
            const speed = Math.floor(data.transferred / elapsed);
            this.progressInfo.speed = this.formatFileSize(speed) + '/s';
            this.lastSpeedUpdate = now;
          }
        }
      });

      // 监听下载进度
      EventsOn('file-download-progress', (data) => {
        const percent = Math.floor(data.percent);
        this.progressPercent = percent;
        this.progressInfo.transferred = this.formatFileSize(data.transferred);
        // 只在第一次更新 total，避免重复计算
        if (!this.progressInfo.total || data.total > 0) {
          this.progressInfo.total = this.formatFileSize(data.total);
        }

        // 节流速度计算，每 500ms 更新一次
        const now = Date.now();
        if (this.progressStartTime && (now - this.lastSpeedUpdate > 500 || data.percent === 100)) {
          const elapsed = (now - this.progressStartTime) / 1000;
          if (elapsed > 0) {
            const speed = Math.floor(data.transferred / elapsed);
            this.progressInfo.speed = this.formatFileSize(speed) + '/s';
            this.lastSpeedUpdate = now;
          }
        }
      });
    },

    async initializeSFTP() {
      try {
        const result = await CreateSFTPClient(this.serverId);
        console.log('SFTP客户端创建结果:', result);
      } catch (error) {
        console.error('创建SFTP客户端失败:', error);
        this.$message.error(`创建SFTP客户端失败: ${error.message}`);
      }
    },

    async loadFileList(path = this.currentPath) {
      this.loading = true;
      try {
        console.log('Loading files from path:', path);
        const files = await ListDirectory(this.serverId, path);
        console.log('Loaded files count:', files ? files.length : 0);
        this.currentPath = path;
        this.pathInput = path;
        this.fileList = files || []; // 确保即使返回null也设置为空数组

        // 如果文件列表为空，显示提示信息
        if (!files || files.length === 0) {
          console.log('No files found in directory');
        }
      } catch (error) {
        console.error('加载文件列表失败:', error);
        this.$message.error(`加载文件列表失败: ${error.message}`);
        this.fileList = []; // 出错时也设置为空数组
      } finally {
        this.loading = false;
      }
    },

    async refreshFileList() {
      await this.loadFileList(this.currentPath);
    },

    handleFileClick(file) {
      if (file.type === 'dir') {
        this.loadFileList(file.path);
      }
    },

    async goToParentDirectory() {
      if (this.isRootDirectory) return;

      // 获取父目录路径
      const parentPath = this.currentPath.substring(0, this.currentPath.lastIndexOf('/'));
      if (parentPath === '') {
        await this.loadFileList('/');
      } else {
        await this.loadFileList(parentPath);
      }
    },

    async navigateToPath(path) {
      if (!path) return;

      // 确保路径以/开头
      if (!path.startsWith('/')) {
        path = '/' + path;
      }

      await this.loadFileList(path);
    },

    selectAndUploadFile() {
      // 直接调用文件选择对话框让用户选择要上传的文件
      this.selectFileToUpload();
    },

    async selectFileToUpload() {
      try {
        // 使用Wails的文件选择对话框让用户选择要上传的文件
        const { OpenFileDialog } = window.go.main.App;
        const localPath = await OpenFileDialog('选择要上传的文件', [
          { displayName: 'All Files', pattern: '*' }
        ]);

        if (localPath) {
          // 从文件路径中提取文件名
          const fileName = localPath.split('\\').pop().split('/').pop();
          const remotePath = `${this.currentPath}/${fileName}`;

          await this.uploadWithProgress(localPath, remotePath, fileName);
        }
      } catch (error) {
        console.error('上传文件失败:', error);
        this.$message.error(`上传文件失败: ${error.message}`);
      }
    },

    async uploadWithProgress(localPath, remotePath, fileName) {
      this.uploading = true;
      this.progressModalVisible = true;
      this.progressTitle = '上传文件';
      this.progressPercent = 0;
      this.progressStatus = 'active';
      this.progressInfo = {
        fileName: fileName,
        transferred: '0 B',
        total: '0 B',
        speed: '0 B/s'
      };
      this.progressStartTime = Date.now();

      try {
        const result = await UploadFileWithProgress(this.serverId, localPath, remotePath);

        this.progressPercent = 100;
        this.progressStatus = 'success';

        // 短暂延迟后关闭进度条
        setTimeout(() => {
          this.progressModalVisible = false;
        }, 1000);

        this.$message.success(result);
        await this.refreshFileList();
      } catch (error) {
        this.progressStatus = 'exception';
        setTimeout(() => {
          this.progressModalVisible = false;
        }, 2000);
        throw error;
      } finally {
        this.uploading = false;
        this.progressStartTime = null;
      }
    },

    onFileSelected(event) {
      // 这个方法现在不会被调用，因为我们直接使用Wails对话框
      const file = event.target.files[0];
      if (file) {
        // 这里是为了兼容性保留的方法
        event.target.value = '';
      }
    },

    async uploadFile(file) {
      // 这个方法现在不会被调用，因为我们直接使用Wails对话框
      // 保留此方法以避免破坏其他可能的调用
    },

    async downloadFile(file) {
      try {
        // 使用Wails的文件保存对话框让用户选择保存位置
        const { SaveFileDialog } = window.go.main.App;
        const localPath = await SaveFileDialog('选择保存位置', file.name);

        if (localPath) {
          await this.downloadWithProgress(file.path, localPath, file.name);
        }
      } catch (error) {
        console.error('下载文件失败:', error);
        this.$message.error(`下载文件失败: ${error.message}`);
      }
    },

    async downloadWithProgress(remotePath, localPath, fileName) {
      this.downloading = true;
      this.progressModalVisible = true;
      this.progressTitle = '下载文件';
      this.progressPercent = 0;
      this.progressStatus = 'active';
      this.progressInfo = {
        fileName: fileName,
        transferred: '0 B',
        total: this.formatFileSize(this.getFileSize(fileName)),
        speed: '0 B/s'
      };
      this.progressStartTime = Date.now();

      try {
        const result = await DownloadFileWithProgress(this.serverId, remotePath, localPath);

        this.progressPercent = 100;
        this.progressStatus = 'success';

        // 短暂延迟后关闭进度条
        setTimeout(() => {
          this.progressModalVisible = false;
        }, 1000);

        this.$message.success(result);
      } catch (error) {
        this.progressStatus = 'exception';
        setTimeout(() => {
          this.progressModalVisible = false;
        }, 2000);
        throw error;
      } finally {
        this.downloading = false;
        this.progressStartTime = null;
      }
    },

    getFileSize(fileName) {
      // 从文件列表中查找文件大小
      const file = this.fileList.find(f => f.name === fileName);
      return file ? file.size : 0;
    },

    showCreateFolderModal() {
      this.folderForm.name = '';
      this.createFolderModalVisible = true;
    },

    async handleCreateFolder() {
      if (!this.folderForm.name.trim()) {
        this.$message.warning('请输入文件夹名称');
        return;
      }

      try {
        const folderPath = `${this.currentPath}/${this.folderForm.name}`;
        const result = await CreateDirectory(this.serverId, folderPath);
        this.$message.success(result);
        this.createFolderModalVisible = false;
        await this.loadFileList(this.currentPath);
      } catch (error) {
        console.error('创建文件夹失败:', error);
        this.$message.error(`创建文件夹失败: ${error.message}`);
      }
    },

    async deleteFile(file) {
      try {
        // 使用 Promise 方式正确处理确认对话框
        await new Promise((resolve, reject) => {
          this.$confirm({
            title: '确认删除',
            content: `确定要删除 ${file.name} 吗？`,
            okText: '确认',
            cancelText: '取消',
            onOk: () => resolve(),
            onCancel: () => reject('cancel')
          });
        });

        const result = await DeleteFile(this.serverId, file.path);
        this.$message.success(result);
        await this.loadFileList(this.currentPath);
      } catch (error) {
        if (error !== 'cancel') {
          console.error('删除文件失败:', error);
          this.$message.error(`删除文件失败: ${error.message}`);
        }
      }
    },

    formatFileSize(size) {
      if (size === 0) return '0 Bytes';
      const k = 1024;
      const sizes = ['Bytes', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(size) / Math.log(k));
      return parseFloat((size / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    },

    formatDate(timestamp) {
      if (!timestamp) return '-';
      const date = new Date(timestamp * 1000);
      return date.toLocaleString('zh-CN');
    }
  }
};
</script>

<style scoped>
.file-manager-container {
  height: 100%;
}

.file-layout {
  height: 100%;
}

.content {
  padding: 16px;
}

.file-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 10px;
}

.path-navigation {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.file-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.file-name {
  cursor: pointer;
  color: #1890ff;
}

.file-name:hover {
  text-decoration: underline;
}

/* 响应式设计 */
@media (max-width: 768px) {
  .file-header {
    flex-direction: column;
    align-items: stretch;
  }

  .path-navigation {
    justify-content: center;
  }

  .file-actions {
    justify-content: center;
  }
}

/* 进度条样式 */
.progress-container {
  padding: 20px 0;
}

.progress-info {
  display: flex;
  justify-content: space-between;
  margin-bottom: 16px;
  font-size: 14px;
}

.progress-speed {
  text-align: center;
  margin-top: 16px;
  font-size: 14px;
}
</style>