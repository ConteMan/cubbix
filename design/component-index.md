# 组件索引

R02 共 31 个 reusable：14 个 Lucide 图标、17 个界面组件。源稿共 107 个 ref；审计包含 children 和 descendants。

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
| Brand placeholder · 待替换为 Logo | `qxNp2` |
| Restart confirmation | `ewQJq` |
| Settings window | `y6tUf` |

## R02 组件变化

- 分段选择按「跟随系统／简体中文／English」排列，母组件默认选中简体中文。
- 设置窗口移除页头与关闭按钮；品牌引用中性文字占位组件，标注「待替换为 Logo」。
- 新增未保存 JSON 文档的重启提示，提供取消和返回文档；不执行隐式丢弃。
- 下载完成提示使用「重启更新」操作；恢复规则见 R02 README。

开关保留开／关／不可用状态；图标按钮保留常态／悬停／键盘焦点及 tooltip；反馈使用 loader／check／alert。卡片的第三行默认隐藏，关于页开启第三行。窗口外壳和卡片使用 ref，页面内容由普通布局框承载。
