# 常亮运行界面验证

本目录为真实 Go 视图的 `ui.Tester` 2× 内存渲染，36 张覆盖中文／英文、浅色／深色：未开启／开启菜单、自定义／截止时刻／跟随窗口、格子／快捷键页，以及录制／冲突态。快捷键拆成独立按键元素，间距为 Space2（2pt），Control 为 U+2303。冲突截图包含悬停 tooltip。

- [格子：中文浅色](tiles-zh-CN-light.png) · [中文深色](tiles-zh-CN-dark.png) · [英文浅色](tiles-en-light.png) · [英文深色](tiles-en-dark.png)
- [快捷键：中文浅色](shortcuts-zh-CN-light.png) · [英文深色](shortcuts-en-dark.png) · [录制](recording-zh-CN-light.png) · [冲突 tooltip](conflict-en-dark.png)
- [自定义](custom-zh-CN-light.png) · [截止时刻](until-en-dark.png) · [跟随进程](follow-zh-CN-light.png)
- [未开启菜单](menu-off-zh-CN-light.png) · [开启菜单](menu-on-en-dark.png)

进程列表使用固定的示例名称与 PID，Terminal／Safari 图标只从系统应用读取，未启动或激活这些应用。运行时列表来自 `ps`，图形应用图标由 `NSRunningApplication` 读取。菜单截图是由同一份 `MenuItem` 模型生成的测试展示，不是 AppKit 菜单外观截图；实际菜单使用 MyGo 原生菜单能力。为避免抢占屏幕，没有打开或拍摄原生菜单，没有发送真实键盘或鼠标事件。

复现截图：在仓库根目录设置 `CUBBIX_AWAKE_UI_EVIDENCE` 为输出目录，执行 `go test -run TestAwakeScreenshots`。普通测试不生成文件。`TestNativeWindowConstraints` 直接启动后台测试进程，窗口全程隐藏，校验三个窗口标题隐藏、标题栏透明、系统按钮隐藏且不可调整大小，以及原生菜单／模板切换和全局快捷键注册后释放。真实 `caffeinate` 测试均有不超过 3 秒的系统超时，并按本次 PID 核对 `pmset -g assertions` 的断言释放；通知仅打桩。
