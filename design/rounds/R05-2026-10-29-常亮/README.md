# 常亮设计 R05 · 第二轮收尾（R05B）

本轮完成普通常亮格子的静态设计，关联 [#2](https://github.com/ConteMan/cubbix/issues/2)，继续在 [PR #13](https://github.com/ConteMan/cubbix/pull/13) 评审。合盖不休眠按 [#4](https://github.com/ConteMan/cubbix/issues/4) 延后。设计事实源为 `design/cubbix.pen`，仅通过 pen.dev headless CLI 0.3.13 编辑、复开与导出。

四张样张：[中文浅色](zh-CN-light.png)、[中文深色](zh-CN-dark.png)、[英文浅色](en-light.png)、[英文深色](en-dark.png)。画布按菜单栏菜单 → 小窗口 → 设置页 → 通知 → 菜单栏图标排列，分区使用小号标签，高度随内容收紧。

## R05B 修改与界面取舍

- **设置页**：格子与快捷键直接引用 R04 Settings window、Settings card、Settings row、Switch、Inset divider 与 Navigation item。窗口宽 720，侧栏 144，内容宽 528、留白 24；行高 48、名称在左、控件在右，行间为内缩分隔线。四项导航需要 232 高，因此两页高度为 `max(176, 卡片自然高度 + 48, 侧栏自然高度)` 后按 4 向上取整，保留与通用／关于相同的组件样式。JSON 使用停用的关闭开关占位。
- **快捷键**：已设置、录制中、冲突三态共用 Shortcut recorder。正常时只显示 ⌃⌥⌘K；录制中在控件内部提示按键，描边仅表示键盘焦点。冲突时只加红色 Lucide alert，说明放在复用 R04 Tooltip 的气泡中，示例为「⌘, 已用于打开设置」／“⌘, opens Settings”。状态比较区展示悬停或键盘聚焦后的 tooltip；离开后隐藏。冲突不弹窗，不覆盖原快捷键。
- **三个小窗口**：无标题栏，仅保留弱化小标签或搜索框；去掉大标题与取消／确定按钮。输入框内以 ↵ 提示回车确认，Esc 取消；空白处可拖动。自定义输入小时和分钟；截止时刻使用 24 小时制。跟随进程按所选 PID 确认，Agent 优先，codex／claude 使用 Lucide terminal；Terminal／Safari 使用灰色圆角「系统应用图标」槽，运行时由系统图标填入，右侧 PID 弱化显示。PID 仅为样例。
- **原生菜单**：半透明背景、背景模糊、圆角与阴影；固定勾选位、右侧子菜单箭头与对齐的快捷键列。未开启、屏幕常亮倒计时、任务常亮、截止时刻、跟随进程和不限时均有示例。模式与时长使用系统菜单条目，不引入自定义开关或输入控件；小窗口从相应菜单项打开。
- **通知**：绘成 macOS 系统通知横幅，包含应用图标、应用名、标题、正文和时间。中文标题「已切换到电池供电」，正文「常亮仍在运行。」；英文标题 “Switched to battery power”，正文 “Keep awake is still active.”。没有应用自定义确认弹窗。
- **菜单栏提案**：由 `brand/dist/menubar-template@2x.png` 的真实 alpha 轮廓派生。脚本填平原来两个眼洞，再切出两道短横线，眼区以外 alpha 逐像素相同。主方案为未开启闭眼、开启睁眼；保留「开启时右下加点」备选。18pt 的 1×／2×、浅／深背景分别输出透明资产和带背景预览；另有 16pt 的同组辨识样张。原尺寸静态观察可区分闭眼与睁眼，但眼缝较细，最终采用哪组仍由维护者定稿。所有提案在 [menubar/](menubar/)，没有写入品牌目录。

## 待决定（本稿先采用的方案）

- 快速开启先记住上次模式与时长；首次为屏幕常亮、1 小时。早于当前时间的截止时刻先解释为次日。仍待确认是否采用这组默认值。
- 跟随选择绑定具体 PID；结束即解除，同名进程或进程重启不自动接续。列表无法读取时禁用确认并以 alert + tooltip 给原因；是否显示更多进程元信息待确认。
- 录制 Esc 恢复原值；已知冲突阻止提交，保留原组合。系统冲突可检测范围和实际键盘焦点行为需要实现阶段验证。
- 改用电池先只通知、继续当前会话；本稿不添加通知操作按钮。通知权限关闭时不再用自定义弹窗打扰；是否增加通知操作待确认。
- 主推闭眼／睁眼，状态点作为微小尺寸备选。菜单栏剩余时长文字及最终品牌轮廓由维护者决定；本稿不改品牌母版。

## 复现与验证

传入固定版本 CLI 的 JavaScript 入口，不依赖桌面端或浏览器：

```sh
node design/scripts/menubar-r05.mjs "$PEN_CLI_FILE"
python3 design/scripts/export-r05.py --cli-file "$PEN_CLI_FILE"
python3 design/scripts/audit.py --out design/rounds/R05-2026-10-29-常亮/audit.json
python3 design/scripts/contrast.py --out design/rounds/R05-2026-10-29-常亮/contrast-validation.json
./scripts/selftest.sh
```

- [设计审计](audit.json)：82 个样式 Token、51 个 reusable、229 个源稿 ref；颜色、圆角、字号、间距写死次数均为 **0**，无缺失变量或悬空引用。新增样式同步至 tokens.json，并通过既有生成器更新 tokens.go；未改变应用交互行为。CLI 无法保留 width／height 变量绑定，几何尺寸使用 Token 解析后的数值，padding／gap 等样式仍保留变量。
- [原生复开检查](render-validation.json)：四张 1× 样张、1,868 个展开节点、0 裁切；已逐张离线目视核对中英文与浅深主题。八个设置窗口实例均为 720 × 232。
- [对比度检查](contrast-validation.json)：活动文字最低 4.55:1，必要图形最低 4.05:1；新增菜单半透明表面按画布背景合成后检查。停用控件和装饰线不计入活动对比度。
- [结构与归档核对](structure-validation.json)：R04 原有节点与归档、brand 文件均保持不变；菜单栏提案 [manifest](menubar/manifest.json) 记录输入和输出摘要，闭眼改动的眼区外像素变化为 0。
- `./scripts/selftest.sh` 全部通过：格式、生成 Token、菜单栏 TIFF、go vet、go test、darwin/arm64 构建与 ad hoc 签名。

此轮没有上传截图附件、没有浏览器检查；以上均为设计和构建证据，尚未验证常亮控制、全局快捷键录制、菜单键盘导航、通知与进程结束的实际运行行为。PR 保持可评审、未合并。
