# R03 · 黑白灰

维护者本轮决定：强调色不使用蓝色或橘色，界面采用黑白灰；橘色只用于应用图标。当前画布已删除 R02 的橘色对比板，R03 不再导出该图。旧轮次目录作为历史记录保持原样。

Logo「猫从箱子里探头」已定稿；按本轮要求仍保留文字占位，下一轮再接入 `brand/dist/mark.svg`。本轮没有导入 Logo 或改动 `brand/`。

## accent 与状态色

| 用途 | 浅色 | 深色 |
| --- | --- | --- |
| AccentFill：开关打开、主要按钮、选中分段 | #202020 | #F5F5F5 |
| AccentOnFill：强调填充上的文字／开关拇指 | #FFFFFF | #202020 |
| AccentSoft：导航选中背景 | #E5E5E5 | #3A3A3A |

`ColorAccent`、`ColorOnAccent`、`ColorAccentSoft` 分别引用上述变量。accent 组仅按 `mode` 区分浅深，不再包含蓝／橘配色值。其他表面、文字、分隔线、tooltip 均改成 R=G=B 的中性色。

失败及阻断状态使用红色 alert。成功 check 选择中性色，复用 `ColorText`：check 形状与短暂停留已经表达成功，额外绿色没有必要，也能让红色保留明确的状态含义。失败原因仍在中性 tooltip 内展示。

导航保留浅灰／深灰底，辅以细中性描边；关闭状态开关的拇指与描边也保持可辨。该描边用于状态识别，键盘焦点另用较粗框且只在键盘操作时出现。

## 对比度

采用 [WCAG 2.2 文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html) 的 sRGB 相对亮度计算，文字保守地统一按 4.5:1 检查；必要图形按 [非文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html) 的 3:1 检查，使用未舍入比值判定。

| 配色对 | 浅色比值 | 深色比值 |
| --- | ---: | ---: |
| 主要按钮／选中分段文字与填充 | 16.29:1 | 14.94:1 |

共检查 60 项活动配色对（26 项文字、34 项图形），全部通过；文字最低 4.77:1，图形最低 4.05:1。停用控件与装饰分隔线不纳入阈值检查。明细及完整精度见 [contrast-validation.json](contrast-validation.json)。这不是运行时键盘、读屏等完整可访问性验收。

## R02 结构保持

窗口仍为固定宽 720、侧栏 144、内容最大宽 528，通用高 176、关于高 316，提示框宽 440。无重复页标题和右上角 ×，仍通过 ⌘W／Esc 关闭。三段语言默认简体中文，恢复与未保存文档提示规则均沿用 R02。

[结构核对](structure-validation.json) 对比 12 张样张的节点类型、组件引用、内容与布局，均与 R02 一致；生成 ID、主题与颜色样式不计入结构差异。[归档校验](archive-validation.json) 确认 R01／R02 共 32 个历史文件字节不变。

## 预览

| 场景 | 中文浅色 | 英文浅色 | 中文深色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 通用 | [PNG](general-zh-CN-light.png) | [PNG](general-en-light.png) | [PNG](general-zh-CN-dark.png) | [PNG](general-en-dark.png) |
| 关于 | [PNG](about-zh-CN-light.png) | [PNG](about-en-light.png) | [PNG](about-zh-CN-dark.png) | [PNG](about-en-dark.png) |
| 未保存文档提示 | [PNG](restart-confirmation-zh-CN-light.png) | [PNG](restart-confirmation-en-light.png) | [PNG](restart-confirmation-zh-CN-dark.png) | [PNG](restart-confirmation-en-dark.png) |

[组件状态](components.png) · [设计基础](foundations.png)。共 14 张 2× PNG，仅导出到本目录，不上传 PR 截图附件，不做浏览器检查。

## Token、组件与写死统计

73 个样式 Token（22 个颜色、46 个数值、5 个字符串），40 个中英文资源键，31 个 reusable（14 个 Lucide 图标、17 个界面组件），95 个源稿 ref。成功色保留语义 Token 名称，但解析为中性色。

| 属性 | 写死次数 | 不同写死值 |
| --- | ---: | ---: |
| 颜色 | 0 | 0 |
| 圆角 | 0 | 0 |
| 字号 | 0 | 0 |
| 间距（padding／gap／letterSpacing） | 0 | 0 |

统计递归包含 children 与 descendants，排除变量定义及几何 x/y/width/height，口径与前两轮一致。[数值与需求审计](audit.json) 通过，无缺失变量、悬空 ref、Token 漂移或未完成占位符。

## 离线验证

[原生复开导出](render-validation.json) 遍历 14 张画板的 938 个展开节点，0 处裁切；[PNG 像素检查](pixel-validation.json) 未发现中性色与语义红之外的颜色，容许两个 RGB 数值级别的栅格舍入差。已离线目视核对主要控件、浅深色与提示状态。

导出、哈希、结构和色值检查均在本地完成。未做浏览器检查、未上传截图附件、未运行应用自测；仓库尚未提供 `scripts/selftest.sh`。本轮只更新设计，PR #10 保持未合并。
