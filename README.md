# Go-Term SSH终端管理器

Go-Term是一个基于Wails框架开发的跨平台SSH终端管理工具，使用Go语言作为后端，Vue.js作为前端，提供了现代化的图形界面来管理和连接远程服务器。
<img width="1356" height="929" alt="image" src="https://github.com/user-attachments/assets/acc4fd41-f7b0-4b6b-919e-af868ea2df14" />

<img width="1364" height="929" alt="image" src="https://github.com/user-attachments/assets/ab58363b-66b9-4c11-aa4a-a1b81dc77c96" />

<img width="1356" height="928" alt="image" src="https://github.com/user-attachments/assets/373a8ef2-5f9a-4a09-ae75-fff8bc831b81" />

## 功能特性

- **多服务器管理**：支持按分组管理多个SSH服务器
- **SSH连接**：支持密码和密钥两种认证方式
- **终端仿真**：基于xterm.js的全功能终端仿真器
- **文件管理**：通过SFTP协议进行远程文件管理
- **自动补全**：智能命令和路径自动补全功能
- **多标签页**：支持同时打开多个终端和文件管理标签页
- **响应式布局**：自适应窗口大小调整

## 技术架构

### 后端技术栈
- **Go语言**：核心业务逻辑
- **Wails框架**：桥接Go后端与前端界面
- **x/crypto/ssh**：SSH协议实现
- **pkg/sftp**：SFTP协议实现

### 前端技术栈
- **Vue.js 3**：前端框架
- **Ant Design Vue**：UI组件库
- **xterm.js**：终端仿真组件
- **Vite**：构建工具

## 项目结构

```
go-term/
├── config/              # 配置文件目录
│   └── servers.json     # 服务器配置文件
├── controllers/         # 控制器层
│   └── ssh_controller.go
├── frontend/            # 前端代码
│   ├── src/
│   │   ├── components/  # Vue组件
│   │   │   ├── FileManager.vue
│   │   │   ├── ServerManager.vue
│   │   │   └── Terminal.vue
│   │   ├── App.vue      # 根组件
│   │   └── main.js      # 入口文件
├── models/              # 数据模型
│   └── server.go
├── services/            # 服务层
│   ├── server_manager.go
│   ├── ssh_service.go
│   └── terminal_session.go
├── app.go               # Wails应用主文件
├── main.go              # 程序入口
├── go.mod               # Go模块定义
└── go.sum               # Go依赖校验和
```

## 核心功能模块

### 1. 服务器管理模块
- 支持服务器分组管理
- 添加、编辑、删除服务器配置
- 支持密码和SSH密钥两种认证方式

### 2. SSH连接模块
- 建立和维护SSH连接
- 执行远程命令
- 创建和管理终端会话

### 3. 终端仿真模块
- 实时显示终端输出
- 支持各种终端控制序列
- 自动调整终端窗口大小

### 4. 文件管理模块
- 浏览远程目录结构
- 上传和下载文件
- 创建和删除目录及文件

### 5. 自动补全模块
- 命令自动补全
- 路径自动补全
- 智能解析补全建议

## 安装与运行

### 环境要求
- Go 1.25+
- Node.js 16+
- npm 或 yarn

### 构建步骤

1. 克隆项目：
```bash
git clone <repository-url>
cd go-term
```

2. 安装前端依赖：
```bash
cd frontend
npm install
cd ..
```

3. 运行开发环境：
```bash
wails dev
```

4. 构建生产版本：
```bash
wails build
```

## 使用指南

### 1. 添加服务器分组
- 点击左侧"服务器分组"区域的"+"按钮
- 输入分组名称
- 点击确认创建分组

### 2. 添加服务器
- 选择一个服务器分组
- 点击"添加服务器"按钮
- 填写服务器信息（名称、主机地址、端口、用户名）
- 选择认证方式（密码或密钥）
- 点击确认添加服务器

### 3. 连接服务器
- 在服务器列表中找到目标服务器
- 点击"连接"按钮建立SSH连接
- 连接成功后状态会显示为绿色"已连接"

### 4. 打开终端
- 确保服务器已连接
- 点击服务器行的"终端"按钮
- 新标签页中将打开终端界面
- 可以像使用普通SSH终端一样执行命令

### 5. 文件管理
- 确保服务器已连接
- 点击服务器行的"文件"按钮
- 新标签页中将打开文件管理界面
- 可以浏览目录、上传下载文件

## 配置文件

`servers.json`文件存储了所有服务器和分组的配置信息，格式如下：

```json
{
  "groups": [
    {
      "id": "group1",
      "name": "分组名称",
      "servers": [
        {
          "id": "server1",
          "name": "服务器名称",
          "host": "主机地址",
          "port": 22,
          "username": "用户名",
          "password": "密码",
          "keyFile": "密钥文件路径",
          "groupId": "所属分组ID"
        }
      ]
    }
  ]
}
```

## 开发说明

### 项目初始化
项目使用Wails CLI工具进行初始化和构建，主要入口点在[main.go](file:///d:/dev/golang/work/go-term/main.go)文件中。

### 数据流设计
1. 前端通过Wails绑定调用后端Go方法
2. 后端处理业务逻辑并通过SSH库与远程服务器通信
3. 实时数据通过Go Channel传递并推送到前端

### 并发安全
- 使用读写锁保护共享资源访问
- 为每个服务器维护独立的互斥锁以避免竞争条件
- 终端会话采用Channel机制实现异步数据传输

## 许可证

本项目采用MIT许可证，详情请参见[LICENSE](file:///d:/dev/golang/work/go-term/LICENSE)文件。
