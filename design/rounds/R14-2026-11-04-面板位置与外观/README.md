# R14：面板位置与外观

按 [规格 005](../../../docs/specs/005-面板位置与外观.md) 追加画板，保留历史根节点；品牌源未改。设计由 `design/scripts/r14.py` 经 headless CLI 0.3.13 写入、复开、导出；`design/scripts/layout.py` 通过（157 个顶层节点，最小间距 80）。

## 复习要点

本轮只出设计，供评审；未实现、未合并。

- **三套边框与阴影方案**（每张画板顶部一行）：A 自绘阴影 + 内外双层描边（推荐）、B 系统阴影 + 单层描边（现状）、C 材质表面 + 外描边。A 的阴影偏移 8、模糊 24、纯黑低透明度，窗口四周因此留 32 白边；B、C 的阴影由系统绘制，画板里的阴影只是示意，半径与透明度调不了。
- **拖动参考线两态**（每张画板左下与中下）：三条参考线为工作区水平中心竖线、默认高度横线、工作区垂直中心横线；进入 8pt 阈值时目标线加粗变实、面板吸附对齐。参考线画在面板下方（画板上按顺序先画面板轮廓、后画虚线示意），以保证观感与实现一致。
- **快捷键页 6 行**（每张画板右下）：新增「卡片面板 ⌃⌥⌘P」一行，内容高 `6 × 48 + 5 × 1 = 293`。

## 设计预览

| 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- |
| [预览](zh-CN-light.png) | [预览](zh-CN-dark.png) | [预览](en-light.png) | [预览](en-dark.png) |

## 新 Token

七个，均已进 `design/cubbix.pen` 变量、`design/tokens.json`、`tokens.go` 与 [Token 索引](../../token-index.md) 的「R14 追加」：

| Token | 类型 | 浅色 | 深色 |
| --- | --- | --- | --- |
| `ColorPanelStrokeOuter` | color | `#00000014` | `#FFFFFF1F` |
| `ColorPanelStrokeInner` | color | `#FFFFFFB3` | `#FFFFFF14` |
| `ColorPanelShadow` | color | `#0000002E` | `#0000007A` |
| `ColorGuideLine` | color | `#2020203D` | `#F5F5F53D` |
| `ColorGuideLineSnap` | color | `#202020CC` | `#F5F5F5CC` |
| `SizePaletteShadowMargin` | number | 32 | 32 |
| `SizePaletteSnap` | number | 8 | 8 |

32 不是取的整数美学，而是与自绘阴影严格对应：窗口留白必须不小于阴影外扩量 `|x| + |y| + blur + spread = 0 + 8 + 24 + 0`。

## 验证

- 导出：`design/scripts/r14.py`，四张 2× PNG，见 [render-validation.json](render-validation.json)（历史根节点未变、四个画板 1132 个节点无布局问题）。
- 布局：`python3 design/scripts/layout.py`。
- `tokens`：`python3 scripts/tokens.py --check`。

画板只验证静态设计值与排布；真实拖动手感、参考线与面板的前后层级、自绘阴影的实际观感、真实壁纸下的对比度、输入法行为均未验证，需在实现后由维护者实测（规格 005 §7.3）。

## 评审意见

待评审。评审结论定稿后补入本节，并把最终选定的方案写回规格 005 的 §5.3。
