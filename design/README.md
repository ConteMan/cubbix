# Cubbix 设计系统 R01

本轮为 Issue #7 提供可评审的设计源、组件库和设置样张。设计源为 [`cubbix.pen`](cubbix.pen)，评审记录与 PNG 在 [R01 目录](rounds/R01-2026-10-28-设计系统/README.md)。本稿待维护者评审，尚未进入界面实现。

- [Token 与 Go 标识符映射](token-index.md)：65 个样式 Token，颜色随浅深主题切换。
- [组件索引](component-index.md)：29 个 reusable，含 14 个 Lucide 图标。
- [语言资源](locales/)：中文和英文各 35 个资源键。
- [图标源文件与许可证](assets/lucide-0.468.0/)：Lucide 固定为 0.468.0。

## 验证与重新导出

在仓库根目录执行：

```sh
python3 design/scripts/audit.py --out design/rounds/R01-2026-10-28-设计系统/audit.json
python3 design/scripts/export.py
```

导出脚本使用已登录的 pen.dev headless CLI 0.3.13，以 `interactive --in … --out …` 重新打开设计源，执行原生布局检查并导出 2× PNG；不连接桌面端。检查结果及设计源、图片的 SHA-256 保存在 `render-validation.json`。

## 交接约定

内容在右侧区域居中，最大宽 528；分组标题位于卡片外，卡片内采用紧凑行与细分隔线。窗口、卡片／条目、控件圆角分别引用 12／8／6 三档 Token。字号、字重、行高、padding、gap 和颜色均引用变量。

CLI 0.3.13 没有保留 `width/height` 的变量绑定，几何尺寸按 Token 解析成数值；文案变量在文本内容中会按字面显示，因此实例从语言资源填入文案。35 个 `Copy…` 变量保留资源映射，`language` 主题同时选择字体；浅深色由 `mode` 主题的颜色变量决定。更改 Token 或文案资源后，应同步设计源、重新审计并导出。

图标按钮的名称与 tooltip 键写在节点 metadata，悬停和键盘聚焦均应可查看。焦点框仅在键盘操作时出现；正常状态不显示标签，进行中使用 loader，成功使用短暂的 check，失败使用 alert 并将原因放入 tooltip。静态稿表达这些状态与约定，不代表运行时交互已经实现。
