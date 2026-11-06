<div align="center">

# Coursive Plugins

**[Coursive](https://github.com/WALKERKILLER/Coursive) 的插件索引、下载镜像列表和官方插件。**

[![CI](https://github.com/WALKERKILLER/coursive-plugins/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/WALKERKILLER/coursive-plugins/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-2f7cf6.svg)](LICENSE)
[![Index schema](https://img.shields.io/badge/index%20schema-v1-44adfe.svg)](index.json)

*[English](README.md) · [插件市场契约](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0025-plugin-marketplace.md) · [来源插件契约](https://github.com/WALKERKILLER/Coursive/blob/main/docs/specs/0024-source-provider-plugins.md) · [参与贡献](CONTRIBUTING.md)*

</div>

> [!NOTE]
> 插件市场契约已在 Coursive 仓库中批准，编号为 0025。Coursive 从 [`index`](https://github.com/WALKERKILLER/coursive-plugins/tree/index) 分支读取签名索引，每次合并后由发布工作流写入。

## 仓库内容

- **[index.json](index.json)**：提交到插件市场的第三方插件，包括版本、各平台的安装包、大小和 SHA-256 哈希。发布工作流会加入官方插件，用 [minisign.pub](minisign.pub) 对应的 Ed25519 密钥签名，再发布到 `index` 分支；Coursive 验证签名后才信任索引。
- **[mirrors.json](mirrors.json)**：Coursive 可以使用的 GitHub 镜像。Coursive 会测速并选择最快的镜像。镜像只负责传输，每次下载都会对照签名索引校验。
- **[plugins/](plugins/)**：官方插件的源代码，每个版本以 `<id>-v<version>` 发布。
- **[sdk/provider](sdk/provider)**：来源插件实现提供者协议所用的 Go SDK。
- **[templates/](templates/)**：助手插件和来源插件的起步模板。
- **[scripts/validate.mjs](scripts/validate.mjs)**：CI 对每次变更运行的检查。

## 插件类型

插件是一个软件包，它的 `package.json` 中带有 `coursive` 清单。一个插件可以同时提供多种功能：

- **助手工具、技能和钩子**：作为 Pi 扩展运行在 Coursive 的助手宿主中。
- **视图和操作**：由应用渲染的交互内容，以及应用中调用插件的按钮。
- **来源**：一个原生的提供程序，负责登录外部课程平台、列出课程和课时、解析媒体地址。凭据由 Coursive 的守护进程保存，下载也由守护进程完成。

## 镜像

| 镜像 | 原始文件 | Release | 源码包 |
| --- | --- | --- | --- |
| GitHub（直连） | ✓ | ✓ | ✓ |
| gh-proxy.com | ✓ | ✓ | ✓ |
| ghfast.top | ✓ | ✓ | ✓ |
| jsDelivr | ✓ | | |
| gh.llkk.cc | ✓ | ✓ | ✓ |
| ghproxy.net | ✓ | | ✓ |

公共镜像随时可能失效。上表的最后检查日期见 `mirrors.json`。发现镜像失效或行为异常，请使用 **Mirror report** 表单报告。不要通过镜像发送令牌或私有地址。

## 发布插件

1. 从 [templates/](templates/) 中的模板开始，在你自己的仓库中开发。
2. 发布一个 GitHub Release，标签与插件版本一致，并上传安装包（一个 `.tgz`；来源插件则每个平台一个）。
3. 提交 Pull Request，在 `index.json` 中加入你的条目。完整的检查清单见 [CONTRIBUTING.md](CONTRIBUTING.md)。

CI 会下载每个新安装包，并核对大小和哈希。维护者审查代码和所申请的权限后才会合并。

## 本地校验

```sh
node scripts/validate.mjs              # 检查格式和规则
node scripts/validate.mjs --artifacts  # 同时下载并校验每个安装包
```

## 安全

索引、镜像列表或官方插件中的安全漏洞，请按 [SECURITY.md](SECURITY.md) 私下报告。

## 许可证

索引、脚本和官方插件以 [MIT 许可证](LICENSE)发布。索引中的第三方插件沿用各自的许可证。

---

<div align="center">

[↑ 回到顶部](#coursive-plugins)

</div>
