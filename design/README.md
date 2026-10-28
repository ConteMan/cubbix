# Cubbix 设计系统 · 常亮 R05B

当前常亮菜单、三个小窗口、格子与快捷键设置、系统通知及菜单栏状态提案见 [R05B 交付](rounds/R05-2026-10-29-常亮/README.md)，覆盖中英文与浅深主题。设置页直接复用 R04 基础组件，当前共 82 个样式 Token、51 个 reusable、104 个中英文资源键。

设计源为 [`cubbix.pen`](cubbix.pen)，R04 基础系统的预览、规则与验证见 [R04 目录](rounds/R04-2026-10-28-收尾/README.md)。R04 在 main 的 Logo 定稿基础上完成单色猫标接入，并移除导航选中描边。R01／R02／R03 目录保留为历史归档。

- [Token 映射](token-index.md)：当前 82 个样式 Token，强调色只按浅深主题解析黑白灰。
- [组件索引](component-index.md)：当前 51 个 reusable、229 个源稿 ref。
- [语言资源](locales/)：中文和英文各 104 个资源键。
- [Lucide 0.468.0 图标与 ISC 许可证](assets/lucide-0.468.0/)；品牌猫标来自 [`mark.svg`](../brand/dist/mark.svg)。

## 品牌与导航

推荐侧栏采用「猫标 + Cubbix」：固定侧栏空间足够，名称帮助识别。猫标使用 20 × 20 标记框，保留源 SVG 比例与负形，跟随 ColorText，浅色墨色、深色反白。标记中心与 16 × 16 导航图标对齐，间隔使用 Space8。仅猫标方案保留为紧凑侧栏备选，两种排法在 [对比板](rounds/R04-2026-10-28-收尾/brand-comparison.png) 中各展示浅深主题。

导航常态、悬停和选中共用 Navigation item。悬停与选中均为内缩圆角色块，尺寸、圆角、padding、gap、字重完全相同，只以灰阶颜色区分，均无描边。ColorNavHover 为浅色 #F3F3F3／深色 #333333；ColorAccentSoft 为浅色 #E0E0E0／深色 #414141。键盘焦点仍只在键盘操作时显示。

## 配色与结构

界面采用黑白灰。主要按钮、打开的开关和选中分段使用近黑／近白强调填充，文字使用 ColorOnAccent。成功 check 使用中性 ColorText；失败或阻断 alert 保留语义红，原因放 tooltip。

窗口固定宽 720、侧栏宽 144、右侧内容最大宽 528 且居中，主内容留白 24。高度取 `max(176, 内容自然高度 + 2 × 24)`，再向上取整到 4 的倍数；通用 720 × 176、关于 720 × 316，提示框宽 440。

不重复通用／关于标题，不显示右上角 ×；保留「更新」分组，使用 ⌘W／Esc 关闭，空白处可拖动。语言为「跟随系统／简体中文／English」，默认简体中文；英文样张展示切换后的状态。

重启更新后，常亮会话按剩余时间恢复；原进程仍运行时继续跟随。有未保存 JSON 文档时先提示，提供取消／返回文档，保留更改后再发起更新。版本号与其他开关状态仅为示例。

## 验证与导出

```sh
python3 design/scripts/audit.py --out design/rounds/R04-2026-10-28-收尾/audit.json
python3 design/scripts/contrast.py --out design/rounds/R04-2026-10-28-收尾/contrast-validation.json
python3 design/scripts/export.py
python3 design/scripts/review.py
```

导出使用 pen.dev headless CLI 0.3.13，保存后复开检查，输出 15 张 2× PNG。`export.py --cli-file` 可复用已有 CLI；`palette.mjs` 参数为同一 CLI 入口与 PNG 目录，使用其 Sharp 依赖离线检查像素。review.py 核对源 SVG 几何、主题色和原生展开的导航状态。

静态对比度按 W3C sRGB 公式计算，活动文字统一以 4.5:1、必要图形以 3:1 为阈值；停用控件和装饰分隔线除外。该检查不代替运行时可访问性验收。

颜色、圆角、字号、字重、行高、padding 与 gap 保留变量。CLI 不保留 width/height 变量绑定，几何尺寸由 Token 解析为数值；文案来自语言资源。R05B 未上传截图附件、未做浏览器检查；仓库现有 `scripts/selftest.sh` 已全部通过，静态交互仍需后续运行验收。PR #13 保持可评审、未合并。
