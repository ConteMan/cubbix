# R10 · 卡片面板

依据[规格 001](../../../docs/specs/001-菜单栏面板.md)，补齐卡片面板评审板，并同步 PR #32 评审修正后的界面。全部通过 pen.dev headless CLI 0.3.13 的 `interactive --in/--out` 编辑、保存、复开和导出；既有 R01–R09 顶层节点未变。

| 主题 | 中文 | 英文 |
| --- | --- | --- |
| 浅色 | [预览](zh-CN-light.png) | [预览](en-light.png) |
| 深色 | [预览](zh-CN-dark.png) | [预览](en-dark.png) |

每张 2× 评审板包含未开启、30 分钟、1 小时、不限时、截止、自定义、跟随、进程结束、JSON 0／3／7 个窗口、7 窗口展开、全关与更新就绪右键菜单，共 14 态。错误行仅保留 alert 图标，原因通过 tooltip 和辅助标签提供；四颗药丸只表达预设，不增加低频入口。画板在 R10 轮次带横向排列，距旧轮次至少 240；顶层节点最小间距通过检查。

评审修正：运行会话携带自身的分钟数与截止时刻，不从记忆默认值或剩余时间猜药丸；URL 不改记忆值，模式热切换保留会话信息，外部动作成功后清理旧错误。Tab 进入选中药丸、方向键仅移动药丸焦点、Enter／Space 执行；模式方向键即时切换。命令面板 tooltip 从当前配置生成，复用 `PaletteShortcutLabel`；删除冗余 `PanelCommandPalette` 和旧菜单 `JSONWindows` 语言键。

`SizeMenubarPanelWidth = 320`、`SizePillButtonWidth = 64`、`SizeIconSmall = 12` 与 `design/tokens.json`／`tokens.go` 一致。颜色、圆角、字号、间距均引用变量。固定版本渲染器会把尺寸变量当作布局字符串，因此宽高几何值由脚本读取 Token 后写入；此限制不涉及颜色和圆角。Lucide plus／command 与 Switch 为复用母版，其他图标复用既有组件。

验证：[设计审计](audit.json)、[复开与导出](render-validation.json)、[防重叠](layout-validation.json)。重导出：`python3 design/scripts/r10.py`。运行渲染见 [runtime/README.md](runtime/README.md)；其 48 张图片通过 `ui.Tester` 在内存生成，错误态已同步更新。

安全边界：完整测试保持后台；隐藏原生测试直接读取／设置 AppKit 策略值，绕开 MyGo 会激活应用的 Regular 包装；注册的测试快捷键均注销，不修改系统 URL 默认处理程序。真实猫标点击、失焦／Esc、全屏 Space、Regular 层级、副屏、中文输入法、Vibrancy／阴影与真实窗口聚焦未实测，不计为通过。
