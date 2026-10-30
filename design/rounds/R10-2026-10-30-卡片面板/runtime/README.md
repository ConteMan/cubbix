# R10 卡片面板 · 运行记录

2026-10-30，随 v0.3.0 第 2 步（[规格 001](../../../../docs/specs/001-菜单栏面板.md)）的实现记录。本轮是**实现证据**，不是设计评审轮：`design/cubbix.pen` 只补了三个新变量，画板与预览图留给评审步骤；R09 记录里「`hideMenubarPanel` 是空实现」的说法由本轮取代。

## 截图

下列 48 张是真实 `menubarPanelView` 的 `ui.Tester` 内存渲染，2× 缩放，宽度取 `SizeMenubarPanelWidth = 320`，高度由 `menubarPanelHeight` 按内容算出。渲染时 `c.Vibrancy()` 为假，因此视图按自己画的表面（`ColorMenuSurface` + `RadiusWindow`）绘制；原生窗口有 Vibrancy 材质时根容器透明，材质由系统绘制。没有创建原生 flyout、没有前台激活、没有真实键盘事件，也没有在屏幕上弹出面板。

| 场景 | 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 未开启（药丸显示记忆项，弱化底色） | [PNG](panel-off-zh-CN-light.png) | [PNG](panel-off-zh-CN-dark.png) | [PNG](panel-off-en-light.png) | [PNG](panel-off-en-dark.png) |
| 30 分钟常亮中 | [PNG](panel-30m-zh-CN-light.png) | [PNG](panel-30m-zh-CN-dark.png) | [PNG](panel-30m-en-light.png) | [PNG](panel-30m-en-dark.png) |
| 1 小时常亮中 | [PNG](panel-1h-zh-CN-light.png) | [PNG](panel-1h-zh-CN-dark.png) | [PNG](panel-1h-en-light.png) | [PNG](panel-1h-en-dark.png) |
| 不限时常亮中 | [PNG](panel-unlimited-zh-CN-light.png) | [PNG](panel-unlimited-zh-CN-dark.png) | [PNG](panel-unlimited-en-light.png) | [PNG](panel-unlimited-en-dark.png) |
| 截止时刻（四颗药丸均不高亮） | [PNG](panel-until-zh-CN-light.png) | [PNG](panel-until-zh-CN-dark.png) | [PNG](panel-until-en-light.png) | [PNG](panel-until-en-dark.png) |
| 跟随进程（仅保持任务） | [PNG](panel-follow-zh-CN-light.png) | [PNG](panel-follow-zh-CN-dark.png) | [PNG](panel-follow-en-light.png) | [PNG](panel-follow-en-dark.png) |
| 目标进程结束（红色告警） | [PNG](panel-error-zh-CN-light.png) | [PNG](panel-error-zh-CN-dark.png) | [PNG](panel-error-en-light.png) | [PNG](panel-error-en-dark.png) |
| JSON 0 个窗口 | [PNG](panel-json-0-zh-CN-light.png) | [PNG](panel-json-0-zh-CN-dark.png) | [PNG](panel-json-0-en-light.png) | [PNG](panel-json-0-en-dark.png) |
| JSON 3 个窗口（首个置顶） | [PNG](panel-json-3-zh-CN-light.png) | [PNG](panel-json-3-zh-CN-dark.png) | [PNG](panel-json-3-en-light.png) | [PNG](panel-json-3-en-dark.png) |
| JSON 7 个窗口（折叠为计数行） | [PNG](panel-json-7-zh-CN-light.png) | [PNG](panel-json-7-zh-CN-dark.png) | [PNG](panel-json-7-en-light.png) | [PNG](panel-json-7-en-dark.png) |
| JSON 7 个窗口（展开） | [PNG](panel-json-7-open-zh-CN-light.png) | [PNG](panel-json-7-open-zh-CN-dark.png) | [PNG](panel-json-7-open-en-light.png) | [PNG](panel-json-7-open-en-dark.png) |
| 两个格子都关闭（极简占位，窗口变矮） | [PNG](panel-empty-zh-CN-light.png) | [PNG](panel-empty-zh-CN-dark.png) | [PNG](panel-empty-en-light.png) | [PNG](panel-empty-en-dark.png) |

截图里的 JSON 窗口名是测试数据（`orders.json` 等），置顶标记是 12pt 的 `SizeIconSmall`。英文文案全部取自 `design/locales/en.json`，无截断折行。字体度量由 `ui.Tester` 决定，不等同于 AppKit 的最终像素。

## 验证边界

单测覆盖：药丸点击即开启并沿用记忆模式、点当前药丸不重置倒计时、总开关停止与按记忆项开启、模式热切换保持剩余时间、状态行五态（剩余／直到／跟随／不限时／进程结束）、药丸高亮规则（4 小时、自定义、截止时刻、跟随进程均不高亮）、JSON 0/3/7 个窗口与折叠展开、窗口行点击收起面板、格子开关与空状态、两个面板互斥（含 ⌥. 双向）、右键菜单条目、面板高度计算。

原生侧只做隐藏检查：由托盘锚点创建 flyout、`SetVisibleOnAllWorkspaces(true)`、内容尺寸随 `SetContentSize` 变化、创建后不显示也不激活（见 `native-result.txt` 与 `go test -run TestNativeWindowConstraints`）。

**未实测**（按要求未在维护者屏幕上弹出面板）：真实猫标点击与位置、外部点击与 Esc 收起、Tooltip 优先吃掉 Esc、全屏 Space 与其他应用全屏时的层级、Regular 激活策略下的表现、多显示器跟随、面板内 Tab 顺序与方向键的真实键盘行为、中文输入法、系统阴影与 Vibrancy 材质观感、真实 JSON 新建与窗口聚焦切换。这些项目在 PR 的验收清单中标为「未实测」，不按通过计算。
