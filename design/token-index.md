# Token 与 Go 标识符映射

当前共 116 个样式 Token（颜色 30、数值 80、字体 6），R04 的 74 项保留；JSON 语法色与分栏尺寸见「R07 追加」，命令面板尺寸见「R09 追加」，卡片面板尺寸见「R10 追加」，快捷键反馈尺寸见「R11 追加」。名称保留为合法 Go 导出标识符；cubbix.pen 为设计事实源，tokens.json 为可审计快照。

## 黑白灰 accent 组

AccentFill、AccentSoft、AccentOnFill 集中在 groups.accent，仅按 mode 定义浅深主题。ColorAccent、ColorAccentSoft、ColorOnAccent 是对应别名；不再提供蓝色或橘色界面变体。

ColorSuccess 引用 ColorText，成功 check 采用中性色。ColorDanger 是唯一允许彩色的语义 Token；其他颜色均为中性 RGB。ColorThumb 在浅色为白、深色为近黑，以对比打开状态的强调填充。

ColorNavHover 是导航悬停背景，ColorAccentSoft 是选中背景；两态共用无描边的内缩圆角形状。SizeBrandMark 为 20，源猫标按比例居中放入该标记框。

## 全量映射

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `ColorWindow` | color | `mode=light: #F5F5F5 / mode=dark: #202020` |
| `ColorSidebar` | color | `mode=light: #EBEBEB / mode=dark: #292929` |
| `ColorCard` | color | `mode=light: #FFFFFF / mode=dark: #2C2C2C` |
| `ColorControl` | color | `mode=light: #F0F0F0 / mode=dark: #3B3B3B` |
| `ColorControlHover` | color | `mode=light: #E6E6E6 / mode=dark: #494949` |
| `ColorNavHover` | color | `mode=light: #F3F3F3 / mode=dark: #333333` |
| `ColorText` | color | `mode=light: #202020 / mode=dark: #F5F5F5` |
| `ColorTextSecondary` | color | `mode=light: #636363 / mode=dark: #B5B5B5` |
| `ColorTextDisabled` | color | `mode=light: #858585 / mode=dark: #929292` |
| `ColorSeparator` | color | `mode=light: #DEDEDE / mode=dark: #474747` |
| `ColorAccent` | color | `$AccentFill` |
| `ColorAccentSoft` | color | `$AccentSoft` |
| `ColorOnAccent` | color | `$AccentOnFill` |
| `ColorThumb` | color | `mode=light: #FFFFFF / mode=dark: #202020` |
| `ColorSuccess` | color | `$ColorText` |
| `ColorDanger` | color | `mode=light: #BC3030 / mode=dark: #FF8D8D` |
| `ColorTooltip` | color | `mode=light: #252525 / mode=dark: #F0F0F0` |
| `ColorTooltipText` | color | `mode=light: #FFFFFF / mode=dark: #202020` |
| `ColorTransparent` | color | `mode=light: #00000000 / mode=dark: #00000000` |
| `FontSizeCaption` | number | `11` |
| `FontSizeSmall` | number | `12` |
| `FontSizeBody` | number | `13` |
| `FontSizeTitle` | number | `20` |
| `FontSizeDisplay` | number | `28` |
| `LineHeightBody` | number | `1.5` |
| `LineHeightTitle` | number | `1.3` |
| `RadiusWindow` | number | `12` |
| `RadiusCard` | number | `8` |
| `RadiusControl` | number | `6` |
| `Space0` | number | `0` |
| `Space2` | number | `2` |
| `Space4` | number | `4` |
| `Space6` | number | `6` |
| `Space8` | number | `8` |
| `Space10` | number | `10` |
| `Space12` | number | `12` |
| `Space16` | number | `16` |
| `Space20` | number | `20` |
| `Space24` | number | `24` |
| `Space32` | number | `32` |
| `Space40` | number | `40` |
| `StrokeHairline` | number | `1` |
| `StrokeIcon` | number | `2` |
| `SizeIcon` | number | `16` |
| `SizeIconLarge` | number | `24` |
| `SizeIconButton` | number | `28` |
| `SizeSwitchWidth` | number | `34` |
| `SizeSwitchHeight` | number | `20` |
| `SizeSwitchThumb` | number | `14` |
| `SizeSidebarWidth` | number | `144` |
| `SizeWindowWidth` | number | `720` |
| `SizeWindowHeight` | number | `176` |
| `SizeContentMaxWidth` | number | `528` |
| `SizeRowHeight` | number | `48` |
| `SizeNavHeight` | number | `32` |
| `SizeSegmentHeight` | number | `28` |
| `SizeMenuWidth` | number | `260` |
| `SizeMenuRowHeight` | number | `32` |
| `SizeButtonHeight` | number | `28` |
| `SizeBrandMark` | number | `20` |
| `OpacityDisabled` | number | `0.45` |
| `FontWeightRegular` | string | `400` |
| `FontWeightMedium` | string | `500` |
| `FontWeightSemibold` | string | `600` |
| `FontFamily` | string | `language=zh-CN: Noto Sans SC / language=en: Inter` |
| `FontFamilyCJK` | string | `Noto Sans SC` |
| `AccentFill` | color | `mode=light: #202020 / mode=dark: #F5F5F5` |
| `AccentSoft` | color | `mode=light: #E0E0E0 / mode=dark: #414141` |
| `AccentOnFill` | color | `mode=light: #FFFFFF / mode=dark: #202020` |
| `ColorOverlay` | color | `mode=light: #00000033 / mode=dark: #00000066` |
| `SizeWindowMinHeight` | number | `176` |
| `SizeWindowHeightAbout` | number | `316` |
| `SizeDialogWidth` | number | `440` |
| `SizeLanguageSelectorWidth` | number | `318` |

## 尺寸与文案

R02 尺寸保持：窗口宽 720，侧栏 144，内容最大宽 528；通用高 176、关于高 316，按内容与上下留白取高后按 Space4 向上取整。提示框宽 440，三段语言选择宽 318。

CLI 0.3.13 不保留 width/height 变量绑定，Size Token 解析为几何数值；颜色、圆角、字号、字重、行高与 padding/gap 保留引用。40 个语言资源键与 Copy 变量作映射，显示文案填入实例，language 主题切换字体。

## R05B 新增

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `ColorMenuSurface` | color | `light: #FFFFFFEB / dark: #2C2C2CEB` |
| `SizeR05MenuWidth` | number | `352` |
| `SizeR05BoardWidth` | number | `1520` |
| `SizeR05SettingsHeight` | number | `232` |
| `SizeR05RecorderWidth` | number | `188` |
| `SizeR05FieldWidth` | number | `164` |
| `SizeMenubar18` | number | `18` |
| `SizeMenubar16` | number | `16` |

## R07 追加

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `ColorSyntaxKey` | color | `light: #A31515 / dark: #FF7B72` |
| `ColorSyntaxString` | color | `light: #0953B7 / dark: #79C0FF` |
| `ColorSyntaxNumber` | color | `light: #097335 / dark: #7EE787` |
| `ColorSyntaxBool` | color | `light: #003388 / dark: #6CB6FF` |
| `ColorSyntaxNull` | color | `light: #003388 / dark: #6CB6FF` |
| `ColorSyntaxIndent` | color | `light: #EBEBEB / dark: #3C3C3C` |
| `SizeJSONTitleInset` | number | `76` |
| `SizeJSONSplitter` | number | `7` |
| `SizeJSONPaneMinWidth` | number | `240` |

ColorSyntax 组是「界面只用黑白灰」的唯一例外，只作用于代码内容；布尔值与 null 共用深蓝，其余四类各自成色，浅深主题下均达到 WCAG AA（最低 5.54:1）。ColorSyntaxIndent 是弱化的分隔线色，只用于缩进参考线。SizeJSONTitleInset 让窗口名称避开系统红绿灯，SizeJSONSplitter 是可拖动的竖向分隔条宽度，SizeJSONPaneMinWidth 是左右两栏各自的最小宽度。

菜单与系统通知的半透明表面共用 ColorMenuSurface；命令面板的宽度、高度、顶部偏移、搜索行、命令行与分组标题高见「R09 追加」。设置窗口高 232 来自四项导航的自然高度（上下 24、品牌 20、分组间距 24、四行 32、三处间隔 4）。菜单栏图标的 18pt 与 16pt 是逻辑尺寸，1×／2× 位图分别解析对应像素密度。新增 Token 已通过 scripts/tokens.py 同步到 tokens.go。

## R09 追加

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `SizePaletteWidth` | number | `640` |
| `SizePaletteHeight` | number | `400` |
| `SizePaletteTopOffset` | number | `96` |
| `SizePaletteSearchHeight` | number | `36` |
| `SizePaletteRowHeight` | number | `36` |
| `SizePaletteGroupHeight` | number | `24` |

命令面板固定 640 × 400，避免随结果数改变窗口尺寸与位置。顶部距工作区 96；搜索行与命令行同高 36；分组标题高 24。颜色全部复用既有 Token：表面 ColorMenuSurface、搜索行 ColorControl、选中 ColorAccentSoft、悬停 ColorNavHover、分组标题 ColorTextDisabled。六个 Token 已同步 `design/tokens.json` 与 `tokens.go`；`cubbix.pen` 变量与画板在评审步骤补齐。

## R10 追加

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `SizeMenubarPanelWidth` | number | `320` |
| `SizePillButtonWidth` | number | `64` |
| `SizeIconSmall` | number | `12` |

菜单栏卡片面板（001）固定宽 320。常亮四颗药丸基准宽 64：`4 × 64 + 3 × 6 = 274`，加卡片内 20 与面板外 24 共 318，余下 2 由 `ui.Grow` 均分。SizeIconSmall 是 JSON 窗口列表置顶标记等紧凑辅助图标尺寸（12）。三者已同步 `design/tokens.json`、`design/cubbix.pen` 变量与 `tokens.go`；R10 画板在评审步骤补齐。

## R11 追加

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `SizeToastSlotWidth` | number | `320` |
| `SizeToastHeight` | number | `36` |
| `SizeFeedbackPositionWidth` | number | `144` |

屏幕提示固定 320 × 36 的透明槽：单行文字 13 × 1.5 ≈ 20，上下各 Space8；胶囊宽度随内容，靠固定槽宽把定位化简为只读 Token，超长文案按「…」截断。设置页「提示位置」的五段分段控件宽 144。颜色全部复用既有 Token：胶囊 ColorMenuSurface、文字 ColorText、关闭态图标 ColorTextSecondary、失败图标 ColorDanger。三个 Token 已同步 `design/tokens.json` 与 `tokens.go`；`cubbix.pen` 变量与 R11 画板在评审步骤补齐（同 R09 的做法）。
