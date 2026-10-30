# R09 命令面板 · 运行记录

2026-10-30。实现依据为 [003 命令面板与 URL scheme](../../../../docs/specs/003-命令面板.md)：⌥. 呼出、搜索与分组、参数语法、⌘K 动作、收藏与最近使用、`cubbix://` scheme，以及设置「快捷键」页的命令面板一行。版本号未修改；卡片面板（001）与快捷键反馈（002）的界面留给后续步骤，`hideMenubarPanel` 目前是显式空实现。

## 截图

下列 24 张是实际 `paletteView` 与设置视图的 `ui.Tester` 内存渲染，2× 缩放，无原生窗口、无前台激活、无真实键盘事件，也没有切换 Space 或弹出面板到屏幕。截图中的 `⌘K` 键帽、分组标题、行内 hint 与错误 alert 都是真实视图绘制。

| 场景 | 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 空查询（收藏／最近使用／全部命令） | [PNG](palette-empty-zh-CN-light.png) | [PNG](palette-empty-zh-CN-dark.png) | [PNG](palette-empty-en-light.png) | [PNG](palette-empty-en-dark.png) |
| 搜索结果（`常亮`／`awake`） | [PNG](palette-search-zh-CN-light.png) | [PNG](palette-search-zh-CN-dark.png) | [PNG](palette-search-en-light.png) | [PNG](palette-search-en-dark.png) |
| 参数输入（`常亮 45m`，hint 45 分钟） | [PNG](palette-params-zh-CN-light.png) | [PNG](palette-params-zh-CN-dark.png) | [PNG](palette-params-en-light.png) | [PNG](palette-params-en-dark.png) |
| 非法参数（`常亮 45x`，两处 alert） | [PNG](palette-invalid-zh-CN-light.png) | [PNG](palette-invalid-zh-CN-dark.png) | [PNG](palette-invalid-en-light.png) | [PNG](palette-invalid-en-dark.png) |
| ⌘K 动作（常亮：模式、时长、直到／自定义／跟随、收藏） | [PNG](palette-actions-zh-CN-light.png) | [PNG](palette-actions-zh-CN-dark.png) | [PNG](palette-actions-en-light.png) | [PNG](palette-actions-en-dark.png) |
| 快捷键页（新增命令面板一行） | [PNG](shortcuts-zh-CN-light.png) | [PNG](shortcuts-zh-CN-dark.png) | [PNG](shortcuts-en-light.png) | [PNG](shortcuts-en-dark.png) |

截图里的收藏与最近使用是测试数据：收藏为「停止常亮」「新建 JSON 窗口」，最近使用为「仅保持任务」「快捷键」；JSON 窗口为两个空白文档。动作列表超过可见高度时滚动，底部收藏动作不在首屏，这是列表的真实行为。快捷键页当前三行（常亮／JSON／命令面板）；窗口高度已按规格的五行为后续两步的反馈方式与提示位置预留，卡片本身按内容高度绘制，因此截图底部留白属于窗口尺寸而非空白行。截图为浅深主题各一份，字形与截断由 `ui.Tester` 的字体度量决定，不等同于 AppKit 字体的最终像素。

## 验证边界

`./scripts/selftest.sh` 覆盖 gofmt、前端产物比较、Token 一致性、设计防重叠、菜单栏 TIFF、vet、全部 Go 单测与 darwin/arm64 打包。新增单测覆盖：`parseAwakeArg` 的全部语法与边界（1、525659、525660）、组合式先于单独单位、`matchScore` 八个层级与多词查询、命令前缀切分（`常亮 1h`／`常亮1h`／`常亮开关`／`json items.json`／共享名 `json`）、分组去重与 JSON 窗口排除、配置默认值与规整、三槽位快捷键冲突、URL 白名单与恶意输入（大小写折叠、首尾斜杠、路径穿越、越界分钟、非法 mode、超长 text）、`json/new` 不读剪贴板、URL 是否改写记忆设置、常亮失败返回 error 且不写设置错误位、收藏与最近使用持久化、位置计算与空结果视图。

**未实测**（均需真实前台，按工作单要求没有在维护者屏幕上弹出面板）：原生 `Panel` 是否拿到键盘、中文输入法候选与上屏、全屏应用与 `Regular` 状态下的层级、系统阴影是否跟随 12 圆角、点击其他菜单栏项目是否收起、`≥` 是否被 ⌥. 占用、`open cubbix://…` 的六条路径与应用未启动时的行为。这些项目在 PR 的验收清单里标为「未实测」，不按通过计算。URL 解析与执行路径已由单测覆盖，测试不调用应用的 `ready` 或系统 URL 注册入口，没有改动系统默认处理程序。


评审补充：运行渲染已按修正后的视图重新生成。新增行为测试覆盖损坏 URL 编码与额外 URL 结构、非法 blank、文本原样传递、异步投递及禁用／退出门禁、常亮参数溢出、失败跨帧保留与重试、⌘K 成功记录最近使用、快捷键别名冲突及录制期间三个槽位注销／恢复。所有快捷键测试均使用内存替身。
