# Cubbix 设计系统 R03 · 黑白灰

维护者决定界面采用黑白灰，橘色仅属于应用图标。设计源为 [`cubbix.pen`](cubbix.pen)，本轮 PNG、规则与验证见 [R03 目录](rounds/R03-2026-10-28-黑白灰/README.md)。当前画布已删除 R02 的橘色对比板；[R01](rounds/R01-2026-10-28-设计系统/README.md) 与 [R02](rounds/R02-2026-10-28-设计系统/README.md) 目录保留为历史归档。

- [Token 映射](token-index.md)：73 个样式 Token，`accent` 组只按浅深主题解析黑白灰。
- [组件索引](component-index.md)：31 个 reusable、95 个源稿 ref。
- [语言资源](locales/)：中文和英文各 40 个资源键。
- [Lucide 0.468.0 图标与 ISC 许可证](assets/lucide-0.468.0/)：品牌仍使用文字占位，下一轮再接入定稿 Logo。

## 配色规则

浅色强调填充为近黑 #202020、填充上文字为白色；深色强调填充为近白 #F5F5F5、填充上文字为 #202020。主要按钮、打开的开关和选中分段使用这组变量。导航选中态采用浅灰／深灰底与中性描边；背景、文字、控件、分隔线和 tooltip 都使用中性灰阶。

失败或阻断状态的 alert 保留红色，原因仍放 tooltip。成功 check 选择中性色：图形与短暂停留已表达成功，不再额外引入绿色；红色因此能集中表达需要处理的状态。除语义红之外，当前画布没有彩色。

选中导航和关闭状态开关的细描边用于识别状态，保持 R02 的尺寸与布局。键盘焦点仍使用独立的较粗焦点框，只在键盘操作时显示。

## R02 结构与产品规则

窗口固定宽 720、侧栏宽 144、右侧内容最大宽 528 且居中，主内容留白 24。高度取 `max(176, 内容自然高度 + 2 × 24)`，再向上取整到 4 的倍数；通用 720 × 176、关于 720 × 316，提示框宽 440。

不重复通用／关于标题，不显示右上角 ×；保留「更新」分组，使用 ⌘W／Esc 关闭，空白处可拖动。语言为「跟随系统／简体中文／English」，默认简体中文；英文样张展示切换后的状态。

重启更新后，常亮会话按剩余时间恢复；原进程仍运行时继续跟随。有未保存 JSON 文档时先提示，当前方案提供取消／返回文档，保留更改后再发起更新。版本号与其他开关状态仅为示例。Logo 已定稿，本轮按工作单继续使用标注「待替换为 Logo」的文字组件。

## 验证与导出

```sh
python3 design/scripts/audit.py --out design/rounds/R03-2026-10-28-黑白灰/audit.json
python3 design/scripts/contrast.py --out design/rounds/R03-2026-10-28-黑白灰/contrast-validation.json
python3 design/scripts/export.py
```

导出使用已登录的 pen.dev headless CLI 0.3.13，保存后以 `interactive --in … --out …` 复开，输出 14 张 2× PNG。导出脚本支持 `--cli-file` 复用已有 CLI；`palette.mjs` 可通过同一 CLI 的 Sharp 依赖离线检查 PNG 像素，参数为 CLI 入口与 PNG 目录。

对比度按 [W3C 文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html) 的 sRGB 公式计算，活动文字统一以 4.5:1 为阈值；必要图形按 [非文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html) 的 3:1 检查。停用控件和装饰分隔线不纳入这些配色对。结果仅覆盖静态色值，不代替运行时可访问性验收。

颜色、圆角、字号、字重、行高、padding 与 gap 保留变量。CLI 不保留 width/height 变量绑定，几何尺寸由 Token 解析为数值；实例文案来自语言资源，Copy 变量保留映射。本轮未上传截图附件、未做浏览器检查或应用自测；仓库尚无 `scripts/selftest.sh`。
