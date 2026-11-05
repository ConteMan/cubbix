# R15：命令面板打磨

按维护者 2026-11-05 的反馈追加画板，保留历史根节点；品牌源未改。设计由 `design/scripts/r15.py` 经 headless CLI 0.3.13 写入、复开、导出；`design/scripts/layout.py` 通过（162 个顶层节点，最小间距 80）。

## 复习要点

- **命令列表**（每张画板左侧）：分类用次要文字跟在标题后面，右侧参数提示保留；列表右侧留出滚动条槽位，行与选中高亮都在槽位左侧结束；搜索行与列表之间不再有分隔线。
- **动作列表**（每张画板中间）：搜索行右上角是 ⌘K 两个键帽；动作行不再重复标分类，只保留标题、勾选态与参数。
- **尺寸与颜色**（每张画板右侧）：键帽、占位符内缩、滚动条槽位三张局部示意，数值与 Token 名同屏写明。

## 设计预览

| 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- |
| [预览](zh-CN-light.png) | [预览](zh-CN-dark.png) | [预览](en-light.png) | [预览](en-dark.png) |

## 新 Token

八个，均已进 `design/cubbix.pen` 变量、`design/tokens.json`、`tokens.go` 与 [Token 索引](../../token-index.md) 的「R15 追加」：

| Token | 类型 | 浅色 | 深色 |
| --- | --- | --- | --- |
| `ColorKeycap` | color | `#FFFFFF` | `#454545` |
| `ColorKeycapStroke` | color | `#00000024` | `#FFFFFF29` |
| `ColorKeycapText` | color | `$ColorTextSecondary` | `$ColorTextSecondary` |
| `SizeKeycapHeight` | number | 20 | 20 |
| `SizeKeycapMinWidth` | number | 20 | 20 |
| `SizeScrollbar` | number | 6 | 6 |
| `SizePaletteScrollbarSlot` | number | 12 | 12 |
| `SizePalettePlaceholderInset` | number | 6 | 6 |

槽位 12 = 条宽 6 + 条右侧 3 + 行与条之间 3；键帽之间 4，内距 6，圆角沿用 `RadiusControl` 6；分类文字沿用 `ColorTextSecondary`，与标题之间留 `Space8`。

## 验证

- 导出：`design/scripts/r15.py`，四张 2× PNG，见 [render-validation.json](render-validation.json)（历史根节点未变、四个画板 512 个节点无布局问题）。
- 布局：`python3 design/scripts/layout.py`。
- `tokens`：`python3 scripts/tokens.py --check`。
- 对比度：`python3 design/scripts/contrast.py`，键帽文字 `ColorKeycapText / ColorKeycap` 浅 6.01:1、深 4.68:1，分类文字在选中行上 `ColorTextSecondary / ColorAccentSoft` 浅 4.55:1、深 4.98:1，均达 WCAG AA。

## 评审意见

实现同 PR 交付；深浅两色、中英两份的界面截图与本文画板一一对应，等维护者验收。

## 实现评审修正

- 分组标题继承面板背景，不重复叠加半透明表面；标题随列表滚动，不吸顶，避免透明标题与下方命令叠字。搜索框在两种主题下均保留 `ColorControl` 底色；动作标题下细线有意保留。现有画板的分组背景与此一致。
- 键帽组件同时复用于快捷键冲突 tooltip 与卡片面板的 JSON／命令面板 tooltip；JSON WebView 工具栏共用相同的键帽 Token，无障碍描述仍提供完整快捷键。
- 修复超长命令标题连分类一起省略的问题：分类与正常时长提示不收缩，标题优先省略；过长的非法参数回显限制在半个面板宽度并省略，避免占满列表。
- 占位文字限制在输入区域内；视觉几何测试覆盖中英文与浅深色，新增空查询组合输入、取消、提交和所有命令分类检查。运行时证据留仓库外。
- 用 headless CLI 将 R15 的 12 处滚动条圆角、160 处间距／内边距改为变量引用，复开导出四张预览；节点数与 R15 以外的根节点不变。滚动条圆角引用 `RadiusControl`，渲染时按条宽夹到半宽。R15 的颜色、字号、圆角与间距写死值为零。
