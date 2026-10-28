# Cubbix 设计系统 R02

本轮按 Issue #7 的维护者评审收敛设置窗口，并补齐语言与更新流程。设计源为 [`cubbix.pen`](cubbix.pen)，当前预览与规则见 [R02 评审目录](rounds/R02-2026-10-28-设计系统/README.md)。设计稿待本轮评审，尚未进入界面实现。

- [Token 与 Go 标识符映射](token-index.md)：73 个样式 Token，强调色集中在 `accent` 组。
- [组件索引](component-index.md)：31 个 reusable，含品牌文字占位和未保存文档提示。
- [语言资源](locales/)：中文和英文各 40 个资源键。
- [图标与 ISC 许可证](assets/lucide-0.468.0/)：Lucide 固定 0.468.0；品牌位置不使用图标。
- [R01 归档](rounds/R01-2026-10-28-设计系统/README.md)：目录保持原样，[R01 源稿](https://github.com/ConteMan/cubbix/blob/580fe96a99f969cbd37d2b207f151d86963d1cd9/design/cubbix.pen) 可按提交查看。

## 尺寸与关闭规则

窗口固定宽 720，侧栏宽 144。右侧内容最大宽 528、居中，上下及两侧留白 24。高度取内容自然高度加上下留白与最小高度 176 中的较大值，再向上取整到 4 的倍数；当前通用页为 720 × 176，关于页为 720 × 316。未保存文档提示居中覆盖于关于页，不扩张设置窗口。

内容区不重复导航标题，也不保留「偏好设置」这类显而易见的分组标题；「更新」保留。无系统标题栏、无右上角 ×，通过 ⌘W 或 Esc 关闭设置，背景空白处可拖动窗口。

## 已确认的产品规则

语言顺序为「跟随系统／简体中文／English」，默认值为「简体中文」；英文样张展示用户切换到 English 后的状态。其他开关的开启状态仅为示例，不代表产品默认策略。

更新重启后，正在进行的常亮会话按剩余时间恢复；跟随进程的会话在原进程仍运行时继续跟随。有未保存的 JSON 文档时，点击「重启更新」先提示。本轮提示方案提供「取消」与「返回文档」：保留当前更改，返回保存后再发起更新。恢复规则由维护者确认，提示按钮方案由本轮设计提出。

## 强调色与品牌占位

默认样张保持蓝色，品牌暂用 Cubbix 文字，组件标注「待替换为 Logo」。`tokens.json` 的 `groups.accent` 集中管理 `AccentFill`、`AccentSoft`、`AccentOnFill`，原 `ColorAccent` 等变量只作别名。Logo 主色定稿后，只改该组 `blue` 默认槽的值，画布节点无需调整。

`accent=orange` 仅用于对比板，呼应维护者提出的橘色系「猫从箱子里探头」方向；不替换默认样张，也不作为已定稿 Logo 色值。

## 验证与重新导出

在仓库根目录执行，输出默认进入 R02，避免覆盖 R01：

```sh
python3 design/scripts/audit.py --out design/rounds/R02-2026-10-28-设计系统/audit.json
python3 design/scripts/export.py
```

导出脚本用已登录的 pen.dev headless CLI 0.3.13，以 `interactive --in … --out …` 复开源稿，检查全部评审画板并导出 2× PNG；不连接桌面端。支持 `--cli-file` 使用已有 CLI 入口。原生检查、设计源与图片 SHA-256 存在 `render-validation.json`。

颜色、圆角、字号、字重、行高、padding 与 gap 保留变量。CLI 0.3.13 不保留 `width/height` 变量绑定，几何尺寸按 Token 解析为数值；文本变量会按字面显示，实例文案由语言资源填入。40 个 `Copy…` 变量保留映射，`language` 主题选择字体。更改资源或 Token 后，应同步源稿并重新审计、导出。

图标按钮均配 tooltip，焦点框仅在键盘操作时出现；进行中用 loader，成功短暂停留 check，失败用 alert，原因放 tooltip。静态稿表达这些状态，不代表运行时行为已实现。仓库尚未提供 `scripts/selftest.sh`，本轮未运行应用自测。
