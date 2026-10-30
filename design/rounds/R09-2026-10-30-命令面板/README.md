# R09 · 命令面板

2026-10-30，随 PR #31 的代码评审同步设计源。依据 [规格 003](../../../docs/specs/003-命令面板.md)，保留既有 R01–R08 的全部顶层节点及变量定义。本轮通过 pen.dev headless CLI 0.3.13 的 `interactive --in/--out` 编辑、保存、复开和导出，没有连接桌面端。

| 主题 | 中文 | 英文 |
| --- | --- | --- |
| 浅色 | [预览](zh-CN-light.png) | [预览](en-light.png) |
| 深色 | [预览](zh-CN-dark.png) | [预览](en-dark.png) |

每张 2× 评审板包含收藏／最近列表、JSON 搜索、`1h30m` 参数、⌘K 动作、空结果、非法参数及快捷键页。面板为 640 × 400，圆角依次引用 RadiusWindow／RadiusCard／RadiusControl；界面使用黑白灰，只有错误图标使用 ColorDanger。六个 SizePalette Token 已同步到设计源。可复用母版包括面板壳层、快捷键页和 Lucide star，既有图标均复用仓库组件。

动作图选取模式、时长、直到和收藏的代表项，便于一屏比较；实际列表顺序、滚动和完整动作以 [ui.Tester 运行渲染](runtime/README.md) 为准。快捷键页展示三槽位冲突示例（面板尝试使用常亮快捷键）；默认面板快捷键仍为 ⌥.。底部两行是规格 002 的目标布局，当前实现只包含三条录制行，窗口已按五行预留高度；本轮未实现反馈服务、反馈控件或卡片面板。

本轮实现规则：URL 只路由白名单；`json/new` 的文本只作为文档正文，不读取剪贴板、文件或执行 `this`。面板命令失败保持打开，错误跨帧保留，输入或选中项变化后可重试；成功的普通命令和 ⌘K 动作进入最近使用，JSON 窗口动作和收藏操作除外。搜索同分时默认项与显示排序一致，共享 `json` 仍默认新建。

验证文件：[设计审计](audit.json)、[复开与导出](render-validation.json)、[防重叠](layout-validation.json)、[对比度](contrast-validation.json)。颜色、圆角、字号和间距写死均为 0；历史顶层节点内容未变。重导出命令为 `python3 design/scripts/r09.py`，它仅通过 CLI 修改原生设计文件；文本资源键保存在文字层名称中，由脚本按主题语言写入实例文案。

真实前台验收仍未进行：Panel 键盘与中文输入法、全屏 Space／Regular 层级、焦点恢复、阴影、其他菜单栏点击、系统 URL 唤起和真实全局快捷键。`hideMenubarPanel` 已位于 `showPalette` 首部，但卡片面板尚未实现，互斥联调留到 001；不将这些项目计为已通过。
