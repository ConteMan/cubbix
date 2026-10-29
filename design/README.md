# Cubbix 设计系统 · JSON R07

JSON 格子的主窗口、this 结果状态、工具取舍、菜单与设置见 [R07 交付](rounds/R07-2026-10-30-JSON/README.md)，覆盖中英文与浅深主题；当前是待评审的设计提案，未实现 JSON 功能。新增设计 Token 单独归档，不接入运行时。

常亮菜单、三个小窗口、格子与快捷键设置、系统通知及菜单栏状态提案见 [R05B 交付](rounds/R05-2026-10-29-常亮/README.md)，覆盖中英文与浅深主题。设置页直接复用 R04 基础组件，R05B 阶段共 82 个样式 Token、53 个 reusable、104 个中英文资源键。R07 后总计 100 个 reusable、452 个源稿 ref，中英文各 179 个资源键；另有 13 个仅用于 JSON 设计的拟议 Token。

设计源为 [`cubbix.pen`](cubbix.pen)，R04 基础系统的预览、规则与验证见 [R04 目录](rounds/R04-2026-10-28-收尾/README.md)。R04 在 main 的 Logo 定稿基础上完成单色猫标接入，并移除导航选中描边。R01／R02／R03 目录保留为历史归档。

- [Token 映射](token-index.md)：当前 82 个样式 Token，强调色只按浅深主题解析黑白灰。
- [组件索引](component-index.md)：当前 100 个 reusable、452 个源稿 ref。
- [语言资源](locales/)：中文和英文各 179 个资源键。
- [Lucide 0.468.0 图标与 ISC 许可证](assets/lucide-0.468.0/)；品牌猫标来自 [`mark.svg`](../brand/dist/mark.svg)。

## 品牌与导航

推荐侧栏采用「猫标 + Cubbix」：固定侧栏空间足够，名称帮助识别。猫标使用 20 × 20 标记框，保留源 SVG 比例与负形，跟随 ColorText，浅色墨色、深色反白。标记中心与 16 × 16 导航图标对齐，间隔使用 Space8。仅猫标方案保留为紧凑侧栏备选，两种排法在 [对比板](rounds/R04-2026-10-28-收尾/brand-comparison.png) 中各展示浅深主题。

导航常态、悬停和选中共用 Navigation item。悬停与选中均为内缩圆角色块，尺寸、圆角、padding、gap、字重完全相同，只以灰阶颜色区分，均无描边。ColorNavHover 为浅色 #F3F3F3／深色 #333333；ColorAccentSoft 为浅色 #E0E0E0／深色 #414141。键盘焦点仍只在键盘操作时显示。

## 配色与结构

界面采用黑白灰。主要按钮、打开的开关和选中分段使用近黑／近白强调填充，文字使用 ColorOnAccent。成功 check 使用中性 ColorText；失败或阻断 alert 保留语义红，原因放 tooltip。

0.0.3 起设置窗口可调整大小，最小 720 × 340；高度覆盖关于页的更新就绪卡片与错误提示（244 + 24 + 48 + 24）。侧栏宽 144 并填满窗口高度，右侧内容最大宽 528 且水平居中，主内容留白 24；页面切换不再改变窗口大小，最后尺寸写入配置。更新内容在右侧子页滚动，Esc 返回关于页。主窗口保留红绿灯，显示时进入 Dock，隐藏／关闭时退回菜单栏常驻。详见 [R06](rounds/R06-2026-10-29-体验修正/README.md)。R02 的 176／316 高度保留为历史设计，提示框宽仍为 440。

不重复通用／关于标题，不显示右上角 ×；保留「更新」分组，使用 ⌘W／Esc 关闭，空白处可拖动。语言为「跟随系统／简体中文／English」，默认简体中文；英文样张展示切换后的状态。

重启更新后，常亮会话按剩余时间恢复；原进程仍运行时继续跟随。有未保存 JSON 文档时先提示，提供取消／返回文档，保留更改后再发起更新。版本号与其他开关状态仅为示例。

## 验证与导出

新画板用 `FindEmptySpace` 或按轮次带放置（同轮从左到右、轮次带间至少 200）；顶层画板及轮次标签之间至少留 80，提交前必须通过 `python3 design/scripts/layout.py` 防重叠检查，`scripts/selftest.sh` 已接入该检查。检查通过 headless CLI 0.3.13 读取实际尺寸，包含自动高度及 reusable ref；可用 `--cli-file` 或 `PEN_CLI_FILE` 复用本地 CLI。

画布从上到下为 R01–R07。R01–R03 仅保留主题标签与带位，历史预览见对应归档目录；现存画板分别排入 R04、R05、R06、R07，不复制历史 PNG 为新画板。

```sh
python3 design/scripts/audit.py --out design/rounds/R04-2026-10-28-收尾/audit.json
python3 design/scripts/contrast.py --out design/rounds/R04-2026-10-28-收尾/contrast-validation.json
python3 design/scripts/export.py
python3 design/scripts/review.py
```

导出使用 pen.dev headless CLI 0.3.13，保存后复开检查，输出 15 张 2× PNG。`export.py --cli-file` 可复用已有 CLI；`palette.mjs` 参数为同一 CLI 入口与 PNG 目录，使用其 Sharp 依赖离线检查像素。review.py 核对源 SVG 几何、主题色和原生展开的导航状态。

静态对比度按 W3C sRGB 公式计算，活动文字统一以 4.5:1、必要图形以 3:1 为阈值；停用控件和装饰分隔线除外。该检查不代替运行时可访问性验收。

颜色、圆角、字号、字重、行高、padding 与 gap 保留变量。CLI 不保留 width/height 变量绑定，几何尺寸由 Token 解析为数值；文案来自语言资源。R05B 未上传截图附件、未做浏览器检查；仓库现有 `scripts/selftest.sh` 已全部通过，静态交互仍需后续运行验收。PR #13 保持可评审、未合并。
