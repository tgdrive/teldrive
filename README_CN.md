# Telegram Drive

Telegram Drive 是一个强大的工具，它允许您使用 Telegram 作为后端来创建自己的云存储服务。

[![Discord](https://img.shields.io/discord/1142377485737148479?label=discord&logo=discord&style=flat-square&logoColor=white)](https://discord.gg/J2gVAZnHfP) 

**点击图标加入 Discord 服务器以获得更好的支持**

[阅读 Wiki 了解常见问题](https://github.com/divyam234/teldrive/wiki).

<details open="open">
  <summary>目录</summary>
  <ol>
    <li>
      <ul>
      <li>
      <a href="#features">特点</a>
    </li>
        <li><a href="#deploy-using-docker-compose">使用 docker-compose 部署</a></li>
       <li><a href="#use-without-docker">无需 Docker 使用</a></li>
      </ul>
    </li>
    <li><a href="#setting-up-things">设置事项</a></li>
    <ul>
      <li><a href="#mandatory-vars">必需变量</a></li>
      <li><a href="#optional-vars">可选变量</a></li>
    </ul>
  </ol>
</details>

## 特点

- **界面:** 基于 Material You 设计，创造美观的用户界面主题。
- **安全:** 您的数据通过 Telegram 的强大加密功能得到安全保护。
- **灵活部署:** 使用 Docker Compose 或无需 Docker 部署。


## 演示

![demo](./public/demo.png)

[用户界面仓库](https://github.com/divyam234/teldrive-ui)


### 使用 docker-compose 部署
首先克隆仓库
```sh
git clone https://github.com/divyam234/teldrive
cd teldrive
touch teldrive.db
```

**按照以下步骤操作**

- 创建 `teldrive.env` 文件，并填入您的变量，然后启动您的容器。

```sh
docker compose up -d
```
- **访问 http://localhost:8080**
- **通过用户界面上传文件会较慢，因为浏览器的限制，请使用 [Teldrive Uploader](https://github.com/divyam234/teldrive-upload) 实现更快的上传速度。如果您正在使用上传器，请确保使用多机器人模式。**

- **如果您打算与他人分享下载链接，请确保启用了带有机器人的多机器人模式。**


### 无需 docker 使用

**按照以下步骤操作**

- 从发布页下载 Teldrive 的可执行文件。


## 设置事项

### 必需变量
在运行机器人之前，您需要设置以下必需变量：

- `APP_ID` : 这是您的 Telegram 账号的 API ID，可以从 my.telegram.org 获得。

- `APP_HASH` : 这是您的 Telegram 账号的 API HASH，可以从 my.telegram.org 获得。

- `JWT_SECRET` : 用于签署 jwt 令牌。

- `DATABASE_URL` : 来自 Postgres 数据库的连接字符串（您可以使用 Neon db 作为 postgres 的免费替代品）。

### 可选变量
除了必需变量，您还可以设置以下可选变量：
- `HTTPS` : 只有当前端在其他域名时才需要。
- `PORT` : 更改监听端口，默认为 8080。
- `ALLOWED_USERS` : 允许某些 Telegram 用户名访问应用，包括您的用户名。在此处输入以逗号分隔的 Telegram 用户名。当您的实例部署在公共云上并且您希望限制其他人访问您的应用时，这个设置是必需的。
- `COOKIE_SAME_SITE` : 只有当前端在其他域名时才需要。

- `LAZY_STREAM_BOTS` : 如果设置为 true，则在流媒体或下载请求结束时启动机器人会话并立即关闭，否则机器人会一直运行直到服务器停止。

- `BG_BOTS_LIMIT` : 如果 `LAZY_STREAM_BOTS` 设置为 false，则最多启动 `BG_BOTS_LIMIT` 个后台机器人，以防止每次请求都重新创建连接（默认值为 10）。


### 使用多机器人支持

> **警告**
> 如果您通过用户界面设置机器人，机器人将自动被添加为频道的管理员，如果失败，请手动添加。


## 常见问题解答

- 如何获取 Postgres 数据库链接？
> 您可以设置本地的 Postgres 实例，但由于备份和数据传输的麻烦，这不是推荐的做法。推荐的方法是使用免费的云端 Postgres 数据库，如 [Neon DB](https://neon.tech/)。


## 贡献

如果您对此项目有任何进一步的想法，欢迎为此项目做出贡献。
