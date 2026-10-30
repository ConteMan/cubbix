# 组件索引

当前 R07 共 100 个 reusable、452 个源稿 ref；R05B 阶段为 53 个 reusable、234 个源稿 ref。R04 基础组件保持原样，新增常亮组件见下方索引；四张双语主题样张见 [R05](rounds/R05-2026-10-29-常亮/README.md)。

R05 新增可复用组件：原生菜单未开启／开启、模式／时长子菜单、自定义时长／时刻／进程窗口，以及格子／快捷键设置页。场景与交互约定见 [R05](rounds/R05-2026-10-29-常亮/README.md)。

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

## R05B 常亮组件

| 组件 | 节点 ID |
| --- | --- |
| R05 Icon/terminal | `O8Imme` |
| R05 Icon/keyboard | `H8TPWq` |
| R05 Icon/search | `GgZ43` |
| R05 Native menu item | `xz4Am` |
| R05 Native menu / off | `RLMny` |
| R05 Native menu / active | `re5DI` |
| R05 Native submenu / mode | `vXGvH` |
| R05 Native submenu / duration | `IuqUj` |
| R05 Native status variants | `DUqvf` |
| R05 Keyboard field | `X88ii6` |
| R05 Shortcut recorder | `Dh9pW` |
| R05 Keyboard shortcut | `NKc7D` |
| R05 Start keep awake tooltip | `MUuwC` |
| R05 Conflict tooltip | `P5KBDV` |
| R05 Dialog / duration | `d2Shh` |
| R05 Dialog / time | `bxxcd` |
| R05 Dialog / process | `n2m80` |
| R05 Settings / tiles | `zKcs0` |
| R05 Settings / shortcuts | `QgsRX` |
| R05 System notification | `wHXVo` |

格子／快捷键页面嵌套引用 R04 设置窗口、卡片、设置行、开关、分隔线和导航项；快捷键三态共用录制器，冲突气泡复用 R04 Tooltip。新增 Lucide terminal／keyboard／search 与现有图标同为 0.468.0，SVG 和 ISC 许可证保存在 assets/lucide-0.468.0。

快捷键显示规则：Keyboard shortcut 的按键为独立文字层，横向间距使用 Space2（2pt），整体垂直居中。既有录制框以相同的 Space2 字距展示组合键，录制提示和数值输入保留 Space0；冲突与开始常亮 tooltip 复用该组件。macOS 原生菜单右侧快捷键不套用此规则。

## R07 JSON 组件

R07 在 R04–R06 基础上新增 47 个 reusable；总计 100 个 reusable、452 个源稿 ref。双语浅深样张与交互取舍见 [R07](rounds/R07-2026-10-30-JSON/README.md)。

| 组件 | 节点 ID |
| --- | --- |
| R07 Icon/pin | `vnA76` |
| R07 Icon/folder-open | `Sr9bx` |
| R07 Icon/copy | `YHZ39` |
| R07 Icon/fold-vertical | `vq9Aj` |
| R07 Icon/unfold-vertical | `k5lTWJ` |
| R07 Icon/message-square-x | `cf6H7` |
| R07 Icon/clipboard-copy | `VqDNy` |
| R07 Icon/text-quote | `VoL1f` |
| R07 Icon/code-xml | `Met7Z` |
| R07 Icon/file-type | `iakjU` |
| R07 Icon/file-input | `i8eMV` |
| R07 Icon/chevron-down | `Lq8Qv` |
| R07 Icon/gauge | `Faq6Y` |
| R07 Icon/clock-3 | `tyiHF` |
| R07 Icon/square | `EUd6P` |
| R07 Native window controls | `rcCKQ` |
| R07 Window toolbar | `hYJPT` |
| R07 Utility toolbar | `kAiAF` |
| R07 this input | `Q4Xx4j` |
| R07 Code line | `hmKwu` |
| R07 JSON editor | `zXt8W` |
| R07 Result panel | `kCDl6` |
| R07 JSON window | `i1Q1N8` |
| R07 Empty window | `CqaJa` |
| R07 State / Array | `NUat9` |
| R07 State / Object | `vZdRC` |
| R07 State / Undefined | `Q39nxd` |
| R07 State / Error | `EJtG6` |
| R07 State / Timeout | `BVTVj` |
| R07 State / Large | `i2RJ2q` |
| R07 Native menu / off | `j22mq` |
| R07 Native menu / active | `XQacm` |
| R07 Settings / tiles | `ik3nL` |
| R07 Settings / shortcuts | `SuszQ` |
| R07 Tool selection | `cocmG` |
| R07 Action / Format | `PByrK` |
| R07 Action / Fold | `Ng6hD` |
| R07 Action / Unfold | `g3HzZW` |
| R07 Action / Comments | `QEYwF` |
| R07 Action / Minify | `VnixZ` |
| R07 Action / Escape | `kIrp3` |
| R07 Action / XML | `Mrqdh` |
| R07 Action / TypeScript | `Vlafs` |
| R07 Action / Pin | `NMhjk` |
| R07 Action / Unpin | `sFk8V` |
| R07 Action / Open | `ICb8f` |
| R07 Action / Copy | `gPAiw` |

工具 Action 母版持有 tooltipKey 与 focus-visible 约定；引用共享 Lucide 图标。JSON window 组合顶部工具区、CodeMirror 编辑器视觉样例、结果面板、固定 this 输入和 8 个工具；错误与超时清空结果，原因只在 tooltip。设置引用基础窗口／设置行／开关／导航；快捷键引用 R05 独立按键与 Space2 间距。
