# R02 · 紧凑设置与更新提示

本轮完成维护者对 R01 的评审修改，在同一设计源和 PR #10 上继续。R01 目录保持不变；本轮预览、统计与验证证据独立归档。

## R01 评审意见与本轮改动

| 维护者意见 | R02 结果 |
| --- | --- |
| 左侧已有通用／关于，不重复大标题；去掉显而易见的分组标题 | 移除内容页大标题和「偏好设置」，保留「更新」分组标题 |
| 移除右上角 × | 删除关闭按钮及页头，通过 ⌘W／Esc 关闭；R01 关闭按钮 tooltip 的评审项随按钮移除一并消除 |
| 840 × 580 过空，缩小或随内容调整高度 | 固定宽 720；通用高 176、关于高 316，内容最大宽 528 |
| Logo 定稿前用中性占位，不用 Lucide 网格作品牌 | 使用可复用的 Cubbix 文字组件，名称和 metadata 均标注「待替换为 Logo」 |
| 默认强调色不变；单列 accent 组，试一版橘色对比 | 默认蓝色保留；三项 accent Token 集中维护，另导出蓝／橘 × 浅／深对比板 |

## 尺寸规则

窗口宽 720、侧栏宽 144，右侧最大内容宽 528 并居中。主内容上下及两侧留白 24。窗口高度为 `max(176, 内容自然高度 + 2 × 24)`，向上取整到 4 的倍数；最小高度覆盖品牌与两项导航。设置行高 48；圆角仍为窗口 12、卡片与条目 8、控件 6。

通用页两行内容加留白约 145，取最小高度 176；关于页包含「更新」组、三行卡片和下载完成提示，内容与留白合计约 314，取高 316。提示层沿用关于页的 720 × 316，提示框宽 440、按文本自然增高。窗口背景空白处仍可拖动。

## 维护者已确认的规则

- 语言选项依次为「跟随系统／简体中文／English」，默认「简体中文」。英文样张是用户切换到 English 后的状态。
- 更新后恢复正在进行的常亮会话，按剩余时间继续；跟随进程的会话在原进程仍运行时继续跟随。
- 存在未保存的 JSON 文档时，点击「重启更新」先显示提示。

本轮提示交互方案提供「取消」与「返回文档」，保留当前更改；返回保存后可再次发起重启更新。按钮方案为本轮设计提案，不把静态样张表述成已实现的保存或恢复逻辑。版本 0.1.0 和开关开启状态仅作示例。

## 强调色组

`tokens.json` 的 `groups.accent` 收纳 `AccentFill`、`AccentSoft`、`AccentOnFill`；`ColorAccent`、`ColorAccentSoft`、`ColorOnAccent` 分别引用它们，焦点、选中态、开关和按钮自动继承。默认样张使用 `accent=blue`；Logo 定色后只需修改这一组的默认槽，无需修改画布节点。

对比板使用同一套组件，另设 `accent=orange`。浅色橘色强调色为 #B84A00，深色为 #FF9A52；对应浅底与前景色也集中在该组。橘色仅为本轮试验，呼应「猫从箱子里探头」的 Logo 方向，不改变当前默认蓝色。

## 预览

| 场景 | 中文浅色 | 英文浅色 | 中文深色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 通用 | [PNG](general-zh-CN-light.png) | [PNG](general-en-light.png) | [PNG](general-zh-CN-dark.png) | [PNG](general-en-dark.png) |
| 关于 | [PNG](about-zh-CN-light.png) | [PNG](about-en-light.png) | [PNG](about-zh-CN-dark.png) | [PNG](about-en-dark.png) |
| 未保存文档提示 | [PNG](restart-confirmation-zh-CN-light.png) | [PNG](restart-confirmation-en-light.png) | [PNG](restart-confirmation-zh-CN-dark.png) | [PNG](restart-confirmation-en-dark.png) |

[蓝色／橘色对比板](accent-comparison.png) · [组件与状态](components.png) · [设计基础](foundations.png)。共 15 张 PNG，均在保存后重新打开设计源，经 headless CLI 导出，分辨率为 2×。

## Token、组件与写死统计

73 个样式 Token：22 个颜色、46 个数值、5 个字符串；40 个中英文文案键及对应 Copy 变量。31 个 reusable：14 个 Lucide 图标、17 个界面组件；新增品牌文字占位与重启提示组件。源稿有 107 个 ref，统计递归包含 `descendants`。详见 [Token 映射](../../token-index.md) 与 [组件索引](../../component-index.md)。

| 属性 | 写死次数 | 不同写死值 |
| --- | ---: | ---: |
| 颜色 | 0 | 0 |
| 圆角 | 0 | 0 |
| 字号 | 0 | 0 |
| 间距（padding／gap／letterSpacing） | 0 | 0 |

口径与 R01 相同：递归检查 `children` 与 `descendants`，排除变量定义及几何 x/y/width/height。CLI 对尺寸、文本变量的兼容处理见 [交接说明](../../README.md)。

## 验证与评审状态

[数值与需求审计](audit.json) 通过；8 张普通样张、4 张提示样张的语言／主题组合齐全，默认蓝色未被橘色替换，无重复页头或关闭按钮、悬空 ref、缺失变量、Token 漂移或未完成占位符。

[原生复开导出](render-validation.json) 检查全部 15 张评审画板，遍历 1142 个展开节点，0 处裁切；已逐张核对中英文排版、三段语言选择、浅深色、提示层和橘色对比。[R01 归档校验](archive-validation.json) 确认原目录 13 个文件字节不变。

仓库尚无 `scripts/selftest.sh`，本轮不包含应用自测、真实键盘关闭、常亮恢复或 JSON 保存流程验收。本轮新尺寸、提示按钮方案和橘色色值待维护者继续评审；未合并、未实现界面。
