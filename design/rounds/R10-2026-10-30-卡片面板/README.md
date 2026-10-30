# R10 · 卡片面板

2026-10-30，随 v0.3.0 第 2 步（[规格 001](../../../docs/specs/001-菜单栏面板.md)）入库。本轮只记录实现证据与三个新 Token；画板、预览图与评审意见留给设计同步步骤，与 R09 的做法一致。

- 运行渲染：[runtime/README.md](runtime/README.md)，48 张中英 × 浅深 `ui.Tester` 内存渲染。
- 变量：`SizeMenubarPanelWidth = 320`、`SizePillButtonWidth = 64`、`SizeIconSmall = 12` 已写进 `design/cubbix.pen` 变量表，并由 `python3 scripts/tokens.py` 同步到 `design/tokens.json` 与 `tokens.go`；映射见 [token-index.md](../../token-index.md) 的「R10 追加」。
- 图标：新增 Lucide 0.468.0 的 `command.svg`（底部命令面板入口）与 `plus.svg`（快速新建 JSON 窗口），其余复用既有资产。
- 文案：新增 `AwakeUntil`、`PanelCommandPalette`、`PanelDuration30m/1h/2h/Inf`、`PanelEmpty`、`PanelJSONWindowsCount`；`R05Mode`、`R05Duration` 随原生菜单移除而删除。

本轮实现规则：左键点猫标开卡片面板，右键弹精简原生菜单（设置…、更新就绪时的重启以完成更新、退出）；卡片只有总开关、两段模式、四颗预设药丸、状态行与 JSON 新建／窗口列表，自定义时长、直到某时刻、跟随进程只在命令面板；卡片与命令面板互斥；面板高度按内容 `SetContentSize`。

R09 记录中「`hideMenubarPanel` 目前是空实现」的说法由本轮取代：它现在关闭卡片面板。
