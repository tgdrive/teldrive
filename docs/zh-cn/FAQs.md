# Teldrive


**目录**
- [Teldrive是什么？](#teldrive是什么)
- [开始使用](#开始使用)
- [Teldrive上传慢](#teldrive上传慢)
- [数据库提供商](#数据库提供商)
- [故障排除](#故障排除)
- [安全与隐私](#安全与隐私)
- [Telegram账户封禁](#telegram账户封禁)
- [存储限制](#存储限制)
- [Teldrive是否安全](#teldrive是否安全)
- [服务关闭](#服务关闭)
- [二进制安装](#二进制安装)
- [teldrive.env详情](#teldriveenv详情)
- [Auth_Key_Unregistered](#auth_key_unregistered)
- [文件大小限制](#文件大小限制)
- [大文件上传](#大文件上传)
- [第三方下载器](#第三方下载器)
- [Rclone配置](#rclone配置)
- [从**Drive迁移](#从drive迁移)
- [访问令牌和API主机](#访问令牌和api主机)
- [持久登录](#持久登录)
- [功能请求](#功能请求)

## Teldrive是什么？

Teldrive是一个利用Telegram无限存储功能来安全存储文件的概念。

## 开始使用

要开始使用Teldrive，你需要：
- 一个用于存储文件的Telegram频道
- 用于上传和下载文件的Telegram机器人
- Teldrive二进制文件（在GitHub上可获得）
- 如果你想远程访问，需要一台服务器。

## Teldrive上传慢

如果你的上传速度慢而且不是互联网问题，考虑使用Teldrive uploader-cli工具或rclone来高效上传文件。

## 数据库提供商

你可以自托管数据库，或者使用neon.tech，这是一个免费的数据库提供商。

## 故障排除

对于常见的问题，如Error 500、Error 400或其他错误，尝试登出并重新登录。如果问题仍未解决，请向开发者报告问题，并确保包括相关日志。

## 安全与隐私

无法保证你的Telegram账户的安全。如果你担心隐私问题，可以在GitHub上审查代码或雇佣一名安全专家。

## Telegram账户封禁

目前为止，还没有因使用Teldrive而导致Telegram账户被封的情况。如果发生在你身上，请与社区分享你的经历。

## 存储限制

Teldrive可以存储Telegram允许的尽可能多的数据。

## Teldrive是否安全

我的Telegram账户安全吗，我怎么知道开发者没有获取我的私人图片？你无法知道。如果你感到偏执，那就阅读GitHub上的代码或者雇佣一个安全专家（可能开发者正在观看你的私人图片）。

## 服务关闭

如果Teldrive关闭或者Telegram停止支持，记住没有免费的午餐。不要在线上存储关键数据，并在不同位置保持多份副本。开发者不对任何数据丢失负责。

## 二进制安装

要使用Teldrive，你需要Teldrive二进制文件和一个名为teldrive.env的文件，它包含了Teldrive运行所需的环境变量。

## teldrive.env详情

有关teldrive.env的更多信息，请访问项目的GitHub仓库。

## Auth_Key_Unregistered

如果你遇到错误401（Auth_Key_Unregistered），检查你的teldrive.env文件。如果你已经更新了Teldrive，请参阅GitHub上的README了解必需的变量。

## 文件大小限制

没有文件大小限制，尽管你可能会在Telegram频道中看到大文件被分块。不用担心；当你下载它们时，它们会被重新构造。

## 大文件上传

由于浏览器限制，Web UI对大文件上传来说不可靠。考虑使用uploader-cli或rclone来上传这些文件。

## 第三方下载器

是的，你可以使用第三方下载器，如aria2、idm、fdm等。

## Rclone配置

要与Teldrive集成Rclone，你需要一个单独的Rclone版本。访问GitHub页面自行编译Rclone。

## 从**Drive迁移

使用Rclone进行迁移。

## 访问令牌和API主机

要找到访问令牌和API主机，请登录到Teldrive（登录URL作为API主机）。登录后，打开开发者工具（通过按F12可以访问）并转到cookies。复制user_session，它作为访问令牌。

## 持久登录

使用最新版本的Chrome或Firefox应该允许持久登录。如果你使用不同的浏览器或设备，你可能需要每次都登录。

## 功能请求

功能请求可能会被考虑，但没有保证。你

可以联系开发者，或者如果你愿意，你可以自己构建这个功能。