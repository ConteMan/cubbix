# M2 JSON · 运行记录

2026-10-30。首版采用维护者确认的 R07 推荐规则：4 个底部工具、全局新建 ⌃⌥⌘J、窗口内置顶 ⌥⌘P、120ms 防抖、每次新 Worker 和 200ms 上限；窗口达到 6 个收进子菜单。大文件阈值、文件及结果上限与实现边界见[项目 README](../../../../README.md#json)。版本号未修改。

## 截图

JSON 图来自真实 **隐藏 WKWebView 的页面快照**，不包含系统红绿灯、窗口阴影或系统菜单。隐藏窗口不持续接收动画帧，截图前等待目标语言／主题 Token 生效，并同步执行固定版本 CodeMirror 的测量阶段，使行号与正文对齐。所有检查均断言窗口保持隐藏、未聚焦，进程未成为前台应用。未使用桌面截图、Computer Use、真实鼠标或键盘输入，也未切换 Space。

设置图是实际应用视图的 `ui.Tester` 内存渲染；菜单图是实际原生菜单模型的测试渲染，不宣称体现 AppKit 的最终像素或焦点行为。文件选择和未保存提示在测试中替换为确定性回调，没有打开系统面板。JSON 快照中的错误／超时 tooltip 由页面内合成 pointer 事件展开。

| 场景 | 中文浅色 | 中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- | --- | --- |
| 数字／有内容 | [PNG](json-number-zh-CN-light.png) | [PNG](json-number-zh-CN-dark.png) | [PNG](json-number-en-light.png) | [PNG](json-number-en-dark.png) |
| 数组 | [PNG](json-array-zh-CN-light.png) | [PNG](json-array-zh-CN-dark.png) | [PNG](json-array-en-light.png) | [PNG](json-array-en-dark.png) |
| 对象 | [PNG](json-object-zh-CN-light.png) | [PNG](json-object-zh-CN-dark.png) | [PNG](json-object-en-light.png) | [PNG](json-object-en-dark.png) |
| undefined | [PNG](json-undefined-zh-CN-light.png) | [PNG](json-undefined-zh-CN-dark.png) | [PNG](json-undefined-en-light.png) | [PNG](json-undefined-en-dark.png) |
| 求值错误 | [PNG](json-error-zh-CN-light.png) | [PNG](json-error-zh-CN-dark.png) | [PNG](json-error-en-light.png) | [PNG](json-error-en-dark.png) |
| 超时 | [PNG](json-timeout-zh-CN-light.png) | [PNG](json-timeout-zh-CN-dark.png) | [PNG](json-timeout-en-light.png) | [PNG](json-timeout-en-dark.png) |
| 空窗口 | [PNG](json-empty-zh-CN-light.png) | [PNG](json-empty-zh-CN-dark.png) | [PNG](json-empty-en-light.png) | [PNG](json-empty-en-dark.png) |
| 大文件 | [PNG](json-large-zh-CN-light.png) | [PNG](json-large-zh-CN-dark.png) | [PNG](json-large-en-light.png) | [PNG](json-large-en-dark.png) |
| 菜单 | [PNG](menu-zh-CN-light.png) | [PNG](menu-zh-CN-dark.png) | [PNG](menu-en-light.png) | [PNG](menu-en-dark.png) |
| 格子设置 | [PNG](tiles-zh-CN-light.png) | [PNG](tiles-zh-CN-dark.png) | [PNG](tiles-en-light.png) | [PNG](tiles-en-dark.png) |
| 快捷键设置 | [PNG](shortcuts-zh-CN-light.png) | [PNG](shortcuts-zh-CN-dark.png) | [PNG](shortcuts-en-light.png) | [PNG](shortcuts-en-dark.png) |

另附[无效 JSON 保留原文](json-invalid-en-dark.png)。编辑区会滚动，截图底部截断的正文行不代表文档丢失；结构结果面板可调高度并独立滚动。

## 验证与性能

`./scripts/selftest.sh` 覆盖锁文件重建一致性、Node 真实 Worker 单测、Go 单测、隐藏 AppKit／WebKit 集成测试、Token 一致性、设计防重叠、TIFF、vet、darwin/arm64 构建与 ad hoc 签名。测试完成后销毁全部测试窗口并退出辅助进程。

隐藏集成测试覆盖数字、数组、对象、undefined、异常、死循环 200ms 中止、恢复、过期任务丢弃、Worker 无 DOM／Go 桥接、折叠／展开、双窗口独立置顶、最小 672 × 220 下的结构结果布局、所选文件读写、未保存取消／保存取消／保存后重启，以及开关后窗口释放与原生快捷键注册／注销。Node 补充字符串、布尔值、null、NaN／Infinity、循环结构、UTF-8 结果上限、取消和大文件边界。配置单测覆盖旧配置迁移与尺寸／开关／快捷键保存；快捷键回归检查修饰键顺序无关的冲突匹配；更新回归检查保存后进入 MyGo 正常退出契约，避免重复保护取消重启。菜单单测覆盖编号、文件名、6 窗口子菜单和关闭后列表更新。

[原始性能数字](performance.json)为隐藏 WebKit 内的实际 CodeMirror 测量：10,494,008 字节／99,003 行，载入 25ms，20 次编辑中位数 14ms、P95 17ms；另一个 10,491,391 字节文件经 Go 读取、桥接、自动格式化及防抖验证约 196ms。64,014 字符单行保持高亮（3ms 载入、2ms 编辑），100,014／120,014／1,000,014 字符单行降级后为 0–1ms。毫秒计时的 0 表示低于计时分辨率。这些数字用于选择保守的 100,000 单行阈值，未测量前台逐帧动画与真实面板交互，也不保证任意 JSON 结构具有同样表现。

复现时将 `CUBBIX_JSON_EVIDENCE` 指向仓库外的空临时目录，运行 `go test -run 'TestJSONHiddenWebKit|TestJSONSettingsAndMenuScreenshots' -v`。辅助进程使用隔离配置；测试不调用应用的显示／聚焦路径，原生快捷键检查使用专用测试组合且随后注销。应用正式的用户点击路径仍正常显示窗口、切换 Dock 策略并聚焦所选窗口。
