# R13：合盖保持

按 004 与现有 token 追加画板，保留历史根节点；品牌源未改。设计由 `design/scripts/r13.py` 经 headless CLI 写入、复开、导出；布局检查通过。

卡片固定顺序：标题、模式、时长、合盖、状态。无内置屏／开发运行／缺 helper 隐藏整行；macOS 12 保留置灰行。恢复按钮只在 004 §3.8 的两种状态出现。MyGo tooltip 不接收指针，含动作的提示采用点击 alert 打开的锚定浮层，点击测试已覆盖；静态说明仍使用 tooltip。浮层使用卡片表面，确保危险按钮在浅深主题都有足够对比度。

## 设计预览

| 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- |
| [预览](zh-CN-light.png) | [预览](zh-CN-dark.png) | [预览](en-light.png) | [预览](en-dark.png) |

## 应用后台截图

以下 56 张均来自真实视图的 `ui.Tester` 2× 内存渲染与注入的假助手；没有显示窗口、抢焦点、授权、安装服务或修改电源设置。截图只验证界面，不能证明实体合盖行为。

| 状态 | 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 未授权 | [zh-CN/light](ui/unregistered-zh-CN-light.png) | [zh-CN/dark](ui/unregistered-zh-CN-dark.png) | [en/light](ui/unregistered-en-light.png) | [en/dark](ui/unregistered-en-dark.png) |
| 可用／关闭 | [zh-CN/light](ui/available-zh-CN-light.png) | [zh-CN/dark](ui/available-zh-CN-dark.png) | [en/light](ui/available-en-light.png) | [en/dark](ui/available-en-dark.png) |
| 开启中 | [zh-CN/light](ui/enabling-zh-CN-light.png) | [zh-CN/dark](ui/enabling-zh-CN-dark.png) | [en/light](ui/enabling-en-light.png) | [en/dark](ui/enabling-en-dark.png) |
| 开启 | [zh-CN/light](ui/on-zh-CN-light.png) | [zh-CN/dark](ui/on-zh-CN-dark.png) | [en/light](ui/on-en-light.png) | [en/dark](ui/on-en-dark.png) |
| 电池开启 | [zh-CN/light](ui/battery-zh-CN-light.png) | [zh-CN/dark](ui/battery-zh-CN-dark.png) | [en/light](ui/battery-en-light.png) | [en/dark](ui/battery-en-dark.png) |
| 低电量拒绝 | [zh-CN/light](ui/low-zh-CN-light.png) | [zh-CN/dark](ui/low-zh-CN-dark.png) | [en/light](ui/low-en-light.png) | [en/dark](ui/low-en-dark.png) |
| 外部设置 | [zh-CN/light](ui/foreign-zh-CN-light.png) | [zh-CN/dark](ui/foreign-zh-CN-dark.png) | [en/light](ui/foreign-en-light.png) | [en/dark](ui/foreign-en-dark.png) |
| 恢复失败＋可点击恢复 | [zh-CN/light](ui/restore-failed-zh-CN-light.png) | [zh-CN/dark](ui/restore-failed-zh-CN-dark.png) | [en/light](ui/restore-failed-en-light.png) | [en/dark](ui/restore-failed-en-dark.png) |
| 需要允许 | [zh-CN/light](ui/approval-zh-CN-light.png) | [zh-CN/dark](ui/approval-zh-CN-dark.png) | [en/light](ui/approval-en-light.png) | [en/dark](ui/approval-en-dark.png) |
| 取消授权 | [zh-CN/light](ui/denied-zh-CN-light.png) | [zh-CN/dark](ui/denied-zh-CN-dark.png) | [en/light](ui/denied-en-light.png) | [en/dark](ui/denied-en-dark.png) |
| 系统不支持 | [zh-CN/light](ui/unsupported-zh-CN-light.png) | [zh-CN/dark](ui/unsupported-zh-CN-dark.png) | [en/light](ui/unsupported-en-light.png) | [en/dark](ui/unsupported-en-dark.png) |
| 我方遗留＋可点击恢复 | [zh-CN/light](ui/stale-zh-CN-light.png) | [zh-CN/dark](ui/stale-zh-CN-dark.png) | [en/light](ui/stale-en-light.png) | [en/dark](ui/stale-en-dark.png) |
| 低电量下限设置 | [zh-CN/light](ui/settings-zh-CN-light.png) | [zh-CN/dark](ui/settings-zh-CN-dark.png) | [en/light](ui/settings-en-light.png) | [en/dark](ui/settings-en-dark.png) |
| 命令动作列表 | [zh-CN/light](ui/palette-zh-CN-light.png) | [zh-CN/dark](ui/palette-zh-CN-dark.png) | [en/light](ui/palette-en-light.png) | [en/dark](ui/palette-en-dark.png) |

复现：`CUBBIX_LID_UI_EVIDENCE="$PWD/design/rounds/R13-2026-11-03-合盖保持/ui" CGO_ENABLED=0 GOTOOLCHAIN=go1.27.1 go test . -run TestLidScreenshots -count=1`。

检查：行顺序、卡片高度、模式与时长不折行、状态省略、图标与开关尺寸、禁用／加载／错误、浮层点击恢复、设置持久化均由截图或交互测试核对。真实系统授权、登录项、实体合盖、全屏／Space 行为未测。
