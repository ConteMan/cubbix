# 组件索引

29 个 reusable：14 个固定版本 Lucide 图标、15 个界面组件。实例数统计包含 `children` 与 `descendants` 中的 ref；运行 `scripts/audit.py` 可重新生成清单。

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
| Settings window | `y6tUf` |

状态约定：开关有开／关／不可用；分段选择有中文／英文选中态；图标按钮有常态／悬停／键盘焦点及 tooltip；状态组件有 loader／check／alert。设置行支持一行可选辅助文字，样张仅在下载完成提示中保留必要说明。

卡片母组件有三个行槽，第三行默认隐藏；「关于」实例开启第三行。窗口实例负责外壳、导航和页标题，页面内容使用独立的普通布局框承载卡片实例，避免嵌套内容插槽在 CLI 保存时改变结构。
