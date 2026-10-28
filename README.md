# Cubbix

> cubby + x：一格一个小工具。

Cubbix 是维护者自用的 macOS 菜单栏工具箱，基于 [MyGo](https://github.com/egoist/mygo) 构建。它把平时只在别的软件里用到一两个功能的场景，集中到一个常驻、轻量、贴合个人习惯的应用里。每个功能是一个独立「格子」，逐个加入，不做插件市场。

## 规划中的格子

| 格子 | 用途 |
| --- | --- |
| 常亮 | 防止休眠：区分「保持屏幕」与「只保持任务」，可设时长、到某时刻或跟随进程结束，快捷键切换 |
| JSON | 多窗口、可置顶的 JSON 查看与编辑，底部 `this` 表达式用 JS 处理数据 |

进度和任务以 GitHub Issues 为准。

## 开发

要求 Go 1.27.1（MyGo 会自动下载工具链），不使用 CGO。

```sh
./scripts/selftest.sh   # 本机完整自测（骨架建立后提供）
```

## 项目资料

- [Agent 规则](AGENTS.md)
- [项目决策](docs/decisions/README.md)
