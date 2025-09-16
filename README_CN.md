# JellyPotBridge
 中文 / [English](README.md)

JellyPotBridge是一个用Go语言开发的工具套件，用于连接Jellyfin媒体服务器和PotPlayer播放器，实现两者之间的无缝集成和播放状态同步。套件包含两部分：后端Go程序和前端油猴脚本。

## 功能特性

### Go后端程序

- 注册自定义URL协议（jellypot://），支持从网页直接启动PotPlayer播放Jellyfin媒体
- 自动从Jellyfin服务器获取媒体信息
- 启动PotPlayer并从上次播放位置继续播放
- 实时监控PotPlayer播放状态（播放/暂停/停止）
- 定期向Jellyfin服务器报告播放进度
- 确保应用程序只有一个实例运行

### Tampermonkey油猴脚本

- 在Jellyfin网页界面添加PotPlayer播放按钮
- 支持直接播放单集影视内容
- 自动处理剧集和季的播放逻辑（剧集播放下一集，季播放第一集）
- 通过jellypot://协议调用后端程序

## 系统要求

- Windows操作系统
- 已安装PotPlayer
- 已部署Jellyfin媒体服务器
- Go 1.24或更高版本（仅开发环境需要）

## 安装

### Go后端程序

#### 编译安装（开发人员）

1. 克隆项目代码

```bash
git clone <项目地址>
cd JellyPotBridge
```

2. 编译项目

```bash
go build -o bin/JellyPotBridge.exe client/JellyPotBridge.go
```

3. 将配置文件复制到bin目录

```bash
copy client/config.yaml bin/
```

#### 直接使用（普通用户）

直接从项目的`bin`目录获取编译好的`JellyPotBridge.exe`和`config.yaml`文件。

### Tampermonkey油猴脚本

1. 安装浏览器扩展：
    -
    Chrome/Edge: [Tampermonkey](https://chrome.google.com/webstore/detail/tampermonkey/dhdgffkkebhmkfjojejmpbldmpobfkfo)
    - Firefox: [Tampermonkey](https://addons.mozilla.org/en-US/firefox/addon/tampermonkey/)

2. 打开Tampermonkey仪表板，点击"添加新脚本"

3. 复制`script/JellyPotBridge.js`文件的全部内容到编辑器中

4. 点击"文件" > "保存"以安装脚本

## 配置

在使用前，需要先配置`config.yaml`文件：

```yaml
reporting-interval: 10s
pot-player-path: "C:\\Program Files\\DAUM\\PotPlayer\\PotPlayerMini64.exe"
jellyfin:
  server-url: http://127.0.0.1:8096
  username: your_username
  password: your_password
  device-id: f7c8a374-365a-4545-94ed-94410338f495
```

- `reporting-interval`: 向Jellyfin服务器报告播放状态的时间间隔
- `pot-player-path`: PotPlayer可执行文件的完整路径
- `jellyfin.server-url`: Jellyfin服务器的URL地址
- `jellyfin.username`: Jellyfin用户名
- `jellyfin.password`: Jellyfin密码
- `jellyfin.device-id`: 设备标识符，保持唯一即可

## 使用方法

### Go后端程序

#### 1. 注册URL协议

首次使用前，需要注册jellypot://协议：

```bash
JellyPotBridge.exe register
```

这将在Windows注册表中注册协议处理器，使得浏览器可以通过jellypot://链接启动应用程序。

#### 2. 播放媒体

注册成功后，可以通过以下方式启动媒体播放：

- 通过浏览器点击jellypot://协议链接，格式为：`jellypot://<item-id>`
- 命令行直接启动：`JellyPotBridge.exe jellypot://<item-id>`

其中`<item-id>`是Jellyfin中媒体项目的唯一标识符。

#### 3. 取消注册URL协议

如果需要取消注册协议，可以执行：

```bash
JellyPotBridge.exe unregister
```

#### 4. 查看帮助信息

```bash
JellyPotBridge.exe help
```

### Tampermonkey油猴脚本

安装脚本后，访问Jellyfin网页界面，在媒体详情页的播放按钮区域会出现一个新的PotPlayer播放按钮：

1. 打开Jellyfin网页界面并登录
2. 导航到任意媒体的详情页
3. 在播放按钮（如"继续"、"播放"）旁边会出现PotPlayer图标按钮
4. 点击该按钮，脚本会自动调用后端程序并启动PotPlayer播放媒体

脚本会根据媒体类型智能处理播放逻辑：

- 对于单集内容：直接播放当前内容
- 对于剧集：自动获取并播放下一集
- 对于季或合集：自动获取并播放第一集

## 注意事项

- 确保PotPlayer已正确安装在配置文件指定的路径
- 确保Jellyfin服务器可访问且凭据正确
- 应用程序运行时会在后台监控PotPlayer，关闭PotPlayer后应用程序也会自动退出
- 配置文件中的密码以明文形式存储，请妥善保管