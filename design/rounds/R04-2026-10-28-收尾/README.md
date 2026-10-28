# R04 · 收尾

Logo PR #9 已合并。本轮先 rebase 到 main，再从 `brand/dist/mark.svg` 接入单色猫标，完成 PR #10 的设计系统收尾；分支历史没有合并提交。本轮只修改 design/，R01／R02／R03 共 53 个历史文件字节不变，见 [归档核对](archive-validation.json)。

## 维护者评审与本轮规则

导航选中态取消 R03 的细描边。悬停和选中共用 Navigation item，均为内缩圆角色块，尺寸、圆角、padding、gap 与字重相同，仅背景和前景颜色表达状态；所有样张的选中覆盖也已清除描边。

| 导航背景 Token | 浅色 | 深色 |
| --- | --- | --- |
| ColorNavHover · 悬停 | #F3F3F3 | #333333 |
| ColorAccentSoft → AccentSoft · 选中 | #E0E0E0 | #414141 |

品牌使用源 SVG 的两个 path 与四个 ellipse，保留 evenodd 的眼睛与箱体负形，不重绘、不带入源 SVG 的偏色墨色。填充统一引用 ColorText，浅色 #202020、深色 #F5F5F5。20 × 20 标记框按源比例居中，与导航 16 × 16 图标中心对齐，品牌名称间隔 Space8。原品牌占位组件与待替换标注已从当前设计删除。

[品牌对比板](brand-comparison.png) 展示「猫标 + Cubbix」和「仅猫标」，两种各含浅深主题。推荐「猫标 + Cubbix」：当前侧栏空间充足，名称能帮助识别；仅猫标保留为紧凑侧栏备选。12 张场景样张均采用推荐方案，组件库另有独立的 Brand icon only。

界面继续采用黑白灰；成功 check 使用 ColorText，失败／阻断 alert 保留语义红。R02 的窗口尺寸、语言选项、更新恢复规则与未保存 JSON 提示沿用，详见 [设计系统规则](../../README.md)。

## 全部预览

| 场景 | 中文浅色 | 英文浅色 | 中文深色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 通用 | [PNG](general-zh-CN-light.png) | [PNG](general-en-light.png) | [PNG](general-zh-CN-dark.png) | [PNG](general-en-dark.png) |
| 关于 | [PNG](about-zh-CN-light.png) | [PNG](about-en-light.png) | [PNG](about-zh-CN-dark.png) | [PNG](about-en-dark.png) |
| 未保存文档提示 | [PNG](restart-confirmation-zh-CN-light.png) | [PNG](restart-confirmation-en-light.png) | [PNG](restart-confirmation-zh-CN-dark.png) | [PNG](restart-confirmation-en-dark.png) |

[设计基础](foundations.png) · [组件状态](components.png) · [品牌两种排法](brand-comparison.png)。共 15 张 2× PNG，仅保存到仓库本轮目录，不上传 PR 截图附件，不做浏览器检查。

## Token、组件与写死统计

74 个样式 Token（23 个颜色、46 个数值、5 个字符串），中英文各 40 个语言资源键；33 个 reusable（14 个 Lucide 图标、1 个品牌猫标、18 个界面组件），110 个源稿 ref。

| 属性 | 写死次数 | 不同写死值 |
| --- | ---: | ---: |
| 颜色 | 0 | 0 |
| 圆角 | 0 | 0 |
| 字号 | 0 | 0 |
| 间距（padding／gap／letterSpacing） | 0 | 0 |

[审计](audit.json) 递归包含 children 与 descendants，排除变量定义及几何 x/y/width/height，口径沿用前轮。颜色、圆角、字号、字重、行高与 padding/gap 保留变量；CLI 不保留 width/height 绑定，几何尺寸由 Token 解析。无缺失变量、悬空 ref、Token 漂移和未完成占位符。

## 离线验证

- [原生复开导出](render-validation.json)：15 张画板、1282 个展开节点，0 处裁切。
- [Logo 来源核对](logo-validation.json)：源文件 SHA-256、6 个图形的路径／坐标映射、evenodd 负形、19 个原生展开标记的尺寸与主题前景色全部通过。
- [导航结构核对](structure-validation.json)：34 个展开导航条目均无描边；悬停与选中尺寸、圆角、padding、gap 相同，背景色不同；12 张样张各含一个猫标与一个选中条目。
- [静态对比度](contrast-validation.json)：66 项活动配色对全部通过，文字最低 4.55:1（阈值 4.5），必要图形最低 4.05:1（阈值 3）。已移除不存在的导航描边检查，加入实际悬停背景配色。停用控件和装饰分隔线除外，不代替运行时可访问性验收。
- [PNG 像素核对](pixel-validation.json)：15 张 PNG 均只含中性灰阶或语义红，容许两个 RGB 数值级别的栅格舍入差。

已离线目视核对全部中英文、浅深主题场景及基础／组件／品牌对比板，猫标负形完整、导航无描边、内容无溢出。未做浏览器检查、未上传截图附件、未运行应用自测；仓库尚无 scripts/selftest.sh。本轮仅推送 PR #10，不合并。
