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
