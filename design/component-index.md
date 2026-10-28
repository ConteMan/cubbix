# 组件索引

R04 共 33 个 reusable：14 个 Lucide 图标、1 个品牌猫标和 18 个界面组件。110 个源稿 ref 包含 descendants 内的嵌套引用。12 张场景样张之外，品牌对比板含浅深主题各两种排法。

| 组件 | 节点 ID |
| --- | --- |
| Icon/blocks | `Ectkj` |
| Icon/settings-2 | `O4KJZC` |
| Icon/info | `uTgqT` |
| Icon/refresh-cw | `YeT0w` |
| Icon/chevron-right | `MTEP9` |
| Icon/sun | `lPkvC` |
| Icon/braces | `Hvppf` |
| Icon/power | `s4LWGi` |
| Icon/check | `xgZOR` |
| Icon/triangle-alert | `sWO1O` |
| Icon/loader-circle | `VFLqP` |
| Icon/x | `QBOEx` |
| Icon/download | `Jy4Q0` |
| Icon/external-link | `RrJvC` |
| Switch | `KfHWu` |
| Segmented control | `Hs9NC` |
| Icon button | `n2ixa` |
| Tooltip | `sFUcN` |
| Text button | `GaYMc` |
| Settings row | `PodLz` |
| Inset divider | `fezUM` |
| Settings card | `X6uV6` |
| Navigation item | `zlRGT` |
| Status icon | `XlhBz` |
| Menu item | `SvwYl` |
| Menu bar dropdown | `zh2Qo` |
| Download ready notice | `BVbwx` |
| Version control | `vUQhl` |
| Restart confirmation | `ewQJq` |
| Logo/Mark | `r2xEJ` |
| Brand lockup | `ivtHk` |
| Brand icon only | `qSiSW` |
| Settings window | `y6tUf` |

## 品牌与状态

- Logo/Mark 精确导入 main 的 `brand/dist/mark.svg`，保留两个 evenodd 路径与四个椭圆；全部填充引用 ColorText。
- Brand lockup 为猫标 + Cubbix，推荐用于当前侧栏；Brand icon only 为紧凑备选。标记框 20 × 20，与 16 × 16 导航图标居中对齐。
- Navigation item 的常态／悬停／选中共用同一形状；悬停用 ColorNavHover，选中用 ColorAccentSoft，均无描边，仅颜色不同。
- 主要按钮和选中分段使用近黑／近白填充，文字使用 ColorOnAccent；打开状态开关使用反差拇指。
- 关闭状态开关使用中性拇指与描边；键盘焦点只在键盘操作时显示。
- 失败／阻断 alert 使用语义红；成功 check 使用中性色。未保存 JSON 提示和恢复规则沿用 R02。
