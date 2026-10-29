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
./scripts/selftest.sh   # vet、测试、设计检查及 ad-hoc 打包；不公证
```

## 发布构建

先提交全部改动，使工作区保持干净。`./scripts/release.sh` 校验 `mygo.json`、应用版本和 `CHANGELOG.md` 的首个 `## x.y.z` 章节一致，运行自测，再由固定版本的 MyGo 生成 Developer ID 签名、公证的应用、DMG、签名更新归档及清单。构建结果位于 `build/`，默认不上传。

发布环境需提前准备 Developer ID 证书、公证钥匙串 profile 及仓库外的更新私钥。私钥文件权限为 0600，可用 `CUBBIX_UPDATE_KEY_PATH` 覆盖脚本的默认路径；脚本只在构建进程中注入 `MYGO_UPDATER_PRIVATE_KEY`，不要开启命令追踪或将密钥写入日志。显式运行 `./scripts/release.sh --upload` 才会上传到草稿 Release，发布由维护者复核后进行。

自动更新在启动时和每 24 小时检查；关闭后取消自动请求，手动检查仍可下载和验签。安装成功只替换磁盘应用，当前进程继续运行，用户通过关于页或菜单栏的重启入口切换版本。菜单栏先采用新增菜单项提示，沿用现有模板图标，尚无持续可见的角标。M0 没有常亮会话或 JSON 文档；后续格子需通过退出保护与会话持久化接入重启流程。

## 项目资料

- [Agent 规则](AGENTS.md)
- [项目决策](docs/decisions/README.md)
