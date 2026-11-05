# R15：命令面板打磨

按维护者 2026-11-05 的反馈追加画板，保留历史根节点；品牌源未改。设计由 `design/scripts/r15.py` 经 headless CLI 0.3.13 写入、复开、导出，第二轮反馈由 `design/scripts/r15-fix.py` 同步到同一排画板；两张脚本都用 `design/scripts/layout.py` 复核间距。

## 复习要点

- **命令列表**（每张画板左侧）：分类比标题小一号，用次要文字跟在标题后面并与标题底部对齐；右侧参数提示保留；行铺满面板内容宽，选中高亮左右各内缩 `Space8`，滚动条落在面板自己的外边距里，不占行宽；搜索行与列表之间不再有分隔线。
- **动作列表**（每张画板中间）：搜索行右上角是 ⌘K 两个键帽；动作行不再重复标分类，只保留标题、勾选态与参数。
- **尺寸与颜色**（每张画板右侧）：键帽、占位符内缩、分类层级、搜索框底色、滚动条位置五张局部示意，数值与 Token 名同屏写明。

## 第二轮反馈（同日）

第一版画板把滚动条留在行右侧的槽位里，维护者指出：不滚动时选中高亮右宽左窄，分类与标题同字号、垂直居中，深色下搜索框和面板分不开，未命名的 JSON 窗口显示成「JSON  JSON」。四处落点：

| # | 反馈 | 落点 |
| --- | --- | --- |
| 1 | 高亮左右不对称，右侧一直为滚动条留槽位 | 列表不再留槽位（`Padding(0, 0, 0, 0)`），高亮左右各 `Space8`；滚动条用负的 `ScrollbarInsets` 移到面板外边距里，`SizePaletteScrollbarOverhang` 9 = 条宽 6 + 间隙 3，高亮右边缘正好是条的左边缘 |
| 2 | 分类要比标题小一号、底部对齐 | 分类 `FontSizeSmall` 12、`ColorTextSecondary`，标题 `FontSizeBody` 13；两者 `AlignItems(Baseline)`，画板用底部对齐的 `Title group` 呈现 |
| 3 | 深色下搜索框与面板太近 | 搜索行单独用 `ColorPaletteSearch`：浅色 #F0F0F0、深色 #464646（占位文字仍达 AA 的最亮档），与深色面板的对比由 1.28:1 提到 1.49:1 |
| 4 | 未命名的 JSON 窗口显示成「JSON  JSON」 | 分类与标题相同时（不分大小写、去首尾空格）不画分类，理由见规格 003 §7.4：标题已经是功能名，再写一遍没有信息，而给这一行改叫「窗口」会让分类列多出一套词 |

## 设计预览

| 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- |
| [预览](zh-CN-light.png) | [预览](zh-CN-dark.png) | [预览](en-light.png) | [预览](en-dark.png) |

## 新 Token

九个，均已进 `design/cubbix.pen` 变量、`design/tokens.json`、`tokens.go` 与 [Token 索引](../../token-index.md) 的「R15 追加」：

| Token | 类型 | 浅色 | 深色 |
| --- | --- | --- | --- |
| `ColorKeycap` | color | `#FFFFFF` | `#454545` |
| `ColorKeycapStroke` | color | `#00000024` | `#FFFFFF29` |
| `ColorKeycapText` | color | `$ColorTextSecondary` | `$ColorTextSecondary` |
| `SizeKeycapHeight` | number | 20 | 20 |
| `SizeKeycapMinWidth` | number | 20 | 20 |
| `SizeScrollbar` | number | 6 | 6 |
| `SizePaletteScrollbarOverhang` | number | 9 | 9 |
| `SizePalettePlaceholderInset` | number | 6 | 6 |
| `ColorPaletteSearch` | color | `#F0F0F0` | `#464646` |

第一版的 `SizePaletteScrollbarSlot`（12）在第二轮退役，槽位不再存在于行宽里。键帽之间 4，内距 6，圆角沿用 `RadiusControl` 6；分类与标题之间留 `Space8`，分类不收缩。

## 验证

- 导出：`design/scripts/r15-fix.py`，四张 2× PNG，见 [render-validation.json](render-validation.json)（历史根节点未变、四个画板 612 个节点无布局问题）。
- 布局：`python3 design/scripts/layout.py`。
- `tokens`：`python3 scripts/tokens.py --check`。
- 对比度：`python3 design/scripts/contrast.py`，键帽文字 `ColorKeycapText / ColorKeycap` 浅 6.01:1、深 4.68:1；分类文字在选中行上 `ColorTextSecondary / ColorAccentSoft` 浅 4.55:1、深 4.98:1；占位文字在搜索框上 `ColorTextSecondary / ColorPaletteSearch` 浅 5.27:1、深 4.60:1，均达 WCAG AA。

## 评审意见

实现同 PR 交付；深浅两色、中英两份的界面截图与本文画板一一对应，等维护者验收。

## 实现评审修正

- 分组标题继承面板背景，不重复叠加半透明表面；标题随列表滚动，不吸顶，避免透明标题与下方命令叠字。搜索框在两种主题下均使用 `ColorPaletteSearch` 底色；动作标题下细线有意保留。现有画板的分组背景与此一致。
- 键帽组件同时复用于快捷键冲突 tooltip 与卡片面板的 JSON／命令面板 tooltip；JSON WebView 工具栏共用相同的键帽 Token，无障碍描述仍提供完整快捷键。
- 修复超长命令标题连分类一起省略的问题：分类与正常时长提示不收缩，标题优先省略；过长的非法参数回显限制在半个面板宽度并省略，避免占满列表。
- 占位文字限制在输入区域内；视觉几何测试覆盖中英文与浅深色，新增对称高亮、滚动条外置、分类字号与基线、搜索框底色、未命名窗口分类五类检查。
- 用 headless CLI 将 R15 的 12 处滚动条圆角、160 处间距／内边距改为变量引用，复开导出四张预览；节点数与 R15 以外的根节点不变。滚动条圆角引用 `RadiusControl`，渲染时按条宽夹到半宽。R15 的颜色、字号、圆角与间距写死值为零。
