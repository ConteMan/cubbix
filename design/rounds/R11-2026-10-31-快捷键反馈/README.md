# R11 快捷键反馈（002）

- 规格：[002 快捷键反馈](../../../docs/specs/002-快捷键反馈.md)。本轮只实现该规格，不改变已合并的 001 卡片面板与 003 命令面板。
- 记录日期：2026-10-31。
- 画板：`design/cubbix.pen` 已同步全部 116 个样式 Token，并新增 R11 一排中英 × 浅深四张评审板；历史画板节点保持不变。由固定版本 `@pen.dev/cli@0.3.13` headless `interactive --in/--out` 编辑、保存、重新打开与 2× 导出。
- 预览：[中文浅色](zh-CN-light.png)、[中文深色](zh-CN-dark.png)、[英文浅色](en-light.png)、[英文深色](en-dark.png)。每张含七种胶囊、五种位置示意、屏幕提示／通知／拒绝降级／关闭四种五行设置页。

## 界面

设置「快捷键」页合并后固定五行：常亮、JSON、命令面板、反馈方式、提示位置。反馈方式为三段（屏幕提示／系统通知／关闭），提示位置为五段（左上／右上／左下／右下／居中），每段画一个 16 × 16 屏幕缩略框和 Space4 实心点。反馈方式不是「这次真的会用屏幕提示」时，整行禁用并降到 OpacityDisabled；系统通知被拒后位置行恢复可改，通知段右侧出现 `triangle-alert`。

屏幕提示是 320 × 36 的透明槽：胶囊宽随内容，靠固定槽宽把定位化简为只读 Token；单行、超长截断，圆角 RadiusWindow 12，背景 ColorMenuSurface。位置距工作区边缘 Space24。

## 截图

`runtime/` 共 68 张 `ui.Tester` 内存渲染（无原生窗口、无真实提示、无通知授权）：

| 组 | 数量 | 说明 |
| --- | --- | --- |
| 屏幕提示文案 | 56 | 7 种状态（1 小时、45 分钟、不限时、直到 02:30、跟随 Google Chrome、已关闭、未能开启）× 右上／左下 × 中英 × 浅深 |
| 设置快捷键页 | 12 | 屏幕提示（位置可改）／系统通知（位置禁用）／通知被拒（位置可改 + alert）× 中英 × 浅深 |

截图由 `CUBBIX_FEEDBACK_UI_EVIDENCE=<目录> go test -run TestFeedbackScreenshots` 生成；设置页两行的文案同时由 `TestFeedbackSettingsRows` 断言。

## 对比度

胶囊文字在 ColorMenuSurface 上按不透明等效色计算（浅色以 #FFFFFF、深色以 #2C2C2C 为底）：

| 前景 | 浅色 | 深色 |
| --- | --- | --- |
| ColorText | 16.29:1 | 12.81:1 |
| ColorTextSecondary（已关闭图标） | 6.01:1 | 6.81:1 |
| ColorDanger（失败图标） | 5.83:1 | 6.28:1 |

均达到 WCAG AA（正文 4.5:1）。

## 未实测

真实屏幕上的出现与淡出、全屏应用之上的层级、鼠标穿透、应用是否被激活与 ⌘Tab 顺序、Mission Control 与窗口循环、五个位置的实际边距、多显示器与拔显示器、设置窗口保持焦点、通知替换与无声音，均未在真实前台实测：本工作单要求测试一律后台，不在真实屏幕弹提示、不调用真实通知 Show。验收清单中这些条目记为未实测，不计为通过。

## 评审修正

- 首帧内连续触发原先会取消空根视图等待，提前构建胶囊；现在只替换待显示文案，保留入场帧。首次显示完整时间为 16 + 150 + 1200 + 166 = 1532 ms。
- 胶囊图标与文字补 `ContentTransition`；进入关闭／正常通知模式会收起已显示的胶囊；改提示位置立即重排当前提示且不重置停留。
- `awake.toggle` URL 返回本次失败，同时仍显示失败反馈，避免调用方把失败当成功。
- 调度器测试断言实际窗口 `ShowInactive`／`Hide`／`Close` 调用与时间；`ui.Tester` 像素测试覆盖真实入场、替换、出场和主题切换，控件点击测试覆盖通知禁用、拒绝恢复可改与配置保存。

## 焦点与通知安全

- 核对 MyGo v0.6.2：`internal/darwin/window.go` 中 Panel 在创建时带 `styleNonactivatingPanel`，并设置 CanJoinAllSpaces、FullScreenAuxiliary、IgnoresCycle、Transient；`ShowInactive` 仅 `orderFrontRegardless`，不调用 `makeKeyAndOrderFront` 或 `activateIgnoringOtherApps`。`SetVisibleOnAllWorkspaces(true)` 保留这些行为位。
- `TestNativeWindowConstraints` 仅创建隐藏的生产反馈窗口，检查 nonactivating、浮动层、所有 Space、全屏辅助、鼠标穿透、无阴影、无可见／key／应用激活状态；不调用显示或焦点 API。现有原生测试注册的快捷键随测试注销。
- 生产通知入口仍为 `mygo.NewNotification(options).Show()`；测试发件器全部替换，不调用真实通知、不请求权限、不改系统默认 URL 处理程序。

## 设计验证

- `render-validation.json`：重新打开导出 4 张，1608 个展开节点无布局问题，附导出哈希；历史根节点完全不变。
- `layout-validation.json`：139 个顶层节点，最小间距 80，新增轮次带与上一轮间距至少 240，防重叠通过。
- `token-audit.json`：颜色／圆角／字号／间距写死各 0，Token 漂移、缺失变量、悬空组件引用和未完成 placeholder 各 0。三个新增尺寸与 `tokens.go` 一致。
- 固定 CLI 的渲染器不解析变量形式的 opacity；禁用透明度从 `OpacityDisabled` 解析为 0.45，几何尺寸同样从 Token 解析。颜色、字号、圆角、padding 与 gap 保持变量引用。
- `./scripts/selftest.sh` 全通过：格式、前端、Token、防重叠、TIFF、vet、全部 Go 测试、darwin/arm64 ad-hoc 签名构建。版本仍为 0.2.1。

上述隐藏窗口属性与 headless 像素检查不代替真实桌面的行为验收；前文「未实测」清单继续适用。本 PR 可评审，未合并、未发布。
