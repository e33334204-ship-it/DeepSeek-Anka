<p align="center">
  <img src="docs/logo.svg" alt="DeepSeek-Anka" width="640"/>
</p>

<p align="center">
  <a href="./README.md">English</a>
  &nbsp;·&nbsp;
  <strong>简体中文</strong>
  &nbsp;·&nbsp;
  <a href="./docs/SPEC.md">Spec</a>
</p>

<h3 align="center">一个 DeepSeek 原生的 AI 编码助手</h3>
<p align="center">基于 Reasonix 内核，集成视觉、UI 自动化、浏览器、IM 四大增强模块。</p>

<br/>

## 🚀 增强功能

基于 Reasonix 内核，集成自 openhanako 的增强模块：

| 模块 | 说明 |
|------|------|
| 🖼️ **视觉 Pipeline** | 纯文本模型自动调用辅助视觉模型分析图片 |
| 🖥️ **Windows UIA** | 完整 UIA 元素树遍历、后台点击/输入/滚动 |
| 🌐 **浏览器增强** | URL 冷保存、健康监控、一次性 Web 搜索 |
| 💬 **IM 桥接** | 飞书 / 微信 / QQ 消息对接 |

## 安装

从 [Releases](https://github.com/e33334204-ship-it/DeepSeek-Anka/releases) 下载安装包。

### 从源码构建

```sh
go build -o bin/deepseek-anka.exe ./cmd/reasonix
```

## 快速开始

```sh
deepseek-anka setup
export DEEPSEEK_API_KEY=sk-...
deepseek-anka chat
```

## License

MIT

## 赞助支持

<p align="center">
  <img src="docs/sponsor-qr.jpg" alt="打赏码" width="240"/>
</p>
