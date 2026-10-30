# AGENTS.md：Cubbix

## 定位

macOS 菜单栏常驻的个人工具箱。每个功能是一个「格子」，各自独立，可以单独开关。只做维护者实际在用的功能，不做插件体系，也不做跨平台。

## 公开仓库规则

本仓库公开。文件、Issue、PR、评论和 commit 信息中不得出现：维护者的其他私有项目名称或链接、本机绝对路径、个人设备与使用环境细节、凭据和账号信息。调研报告、实验程序和本机证据放在仓库外，Issue 里只写结论。

## 语言

- 对话、Issue、PR、文档使用中文；代码、注释、脚本使用英文。
- Commit 使用中文 Conventional Commits。
- 应用界面默认中文，可切换英文。文案从一开始就放进语言资源，不在代码里写死；验收截图中英文各一份。

## 技术约束

- Go 1.27.1，MyGo 固定 v0.6.2；升级需单独提 Issue 并说明理由。
- 不使用 CGO。需要调用系统框架时优先用系统命令（如 `caffeinate`），其次用 purego。
- 编辑器类界面可以用 MyGo 的 WebView 嵌入成熟前端库，库要固定版本并随应用打包，不从网络加载。
- 应用名 `Cubbix.app`，路径和程序名都不能带空格。配置目录 `~/Library/Application Support/cubbix/`。
- 应用不保存凭据。构建和发布用到的签名、公证、更新密钥只通过环境变量注入，不进仓库、不进日志。

## 设计规则

- 新画板用 `FindEmptySpace` 或按轮次带放置（同轮从左到右、轮次带间至少 200）；顶层画板及轮次标签之间至少留 80，提交前必须通过 `python3 design/scripts/layout.py` 防重叠检查（已接入 `scripts/selftest.sh`）。
- 界面设计源是 `design/cubbix.pen`：维护者用 pen.dev 桌面端编辑，Agent 用 pen.dev headless CLI（`interactive --in/--out`）编辑，导出和复开检查均用 CLI。品牌源 `brand/cubbix-logo.pen` 只由维护者在桌面端编辑，CLI 只读取和导出，不改写。每轮评审在 `design/rounds/` 下新建目录，放导出的预览图和 `README.md`（目标、评审意见、定下的规则）。交互原型用 HTML，放在 `design/prototypes/`。
- 画布上的颜色、字号、圆角、间距一律引用变量，不写死数值。
- 界面只用黑白灰，唯一例外是代码内容：JSON 编辑区和结果区里键名、字符串、数字、布尔值、null 使用 `ColorSyntaxKey`／`ColorSyntaxString`／`ColorSyntaxNumber`／`ColorSyntaxBool`／`ColorSyntaxNull` 语法色，缩进参考线用 `ColorSyntaxIndent`。工具栏、按钮、状态与标题等界面元素仍只用黑白灰；语法色在浅深主题下各自达到 WCAG AA。
- 低信息密度：以图标引导为主，不写说明文字；正常状态不打标签，只在异常时提示。
- 图标统一用 Lucide，固定版本，用到的 SVG 连同许可证放进仓库；纯图标按钮配 tooltip，写法如「开始常亮 (⌃⌥⌘K)」。
- 窗口默认无系统标题栏，拖动背景空白处移动窗口；需要系统菜单的主窗口保留红绿灯。
- 焦点框只在键盘操作时显示（focus-visible）；状态反馈只用图标：进行中 loader、成功 check（短暂停留后收起）、失败 alert（原因放 tooltip）。
- 圆角统一三档：窗口 12、卡片和条目 8、控件 6。
- 界面改动先出设计稿，经维护者确认后再实现。

## 验证与合并

- 以本机 `./scripts/selftest.sh` 全部通过为合并条件，并把结果写进 PR。暂不加 GitHub Actions。
- 涉及菜单栏、快捷键、窗口焦点、输入法的改动，要在本机实测并附截图或录屏。
- 临时 GOPATH、gocache、下载的二进制等都放在仓库外的临时目录，完成后删除。
- 合并后立即删除对应 worktree 和本地分支，再执行 `git worktree prune`。
