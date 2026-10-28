# Token 与 Go 标识符映射

R03 共 73 个样式 Token。名称保留为合法 Go 导出标识符；cubbix.pen 为设计事实源，tokens.json 为可审计快照。

## 黑白灰 accent 组

AccentFill、AccentSoft、AccentOnFill 集中在 groups.accent，仅按 mode 定义浅深主题。ColorAccent、ColorAccentSoft、ColorOnAccent 是对应别名；不再提供蓝色或橘色界面变体。

ColorSuccess 引用 ColorText，成功 check 采用中性色。ColorDanger 是唯一允许彩色的语义 Token；其他颜色均为中性 RGB。ColorThumb 在浅色为白、深色为近黑，以对比打开状态的强调填充。

## 全量映射

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `ColorWindow` | color | `mode=light: #F5F5F5 / mode=dark: #202020` |
| `ColorSidebar` | color | `mode=light: #EBEBEB / mode=dark: #292929` |
| `ColorCard` | color | `mode=light: #FFFFFF / mode=dark: #2C2C2C` |
| `ColorControl` | color | `mode=light: #F0F0F0 / mode=dark: #3B3B3B` |
| `ColorControlHover` | color | `mode=light: #E6E6E6 / mode=dark: #494949` |
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
| `SizeBrandMark` | number | `36` |
| `OpacityDisabled` | number | `0.45` |
| `FontWeightRegular` | string | `400` |
| `FontWeightMedium` | string | `500` |
| `FontWeightSemibold` | string | `600` |
| `FontFamily` | string | `language=zh-CN: Noto Sans SC / language=en: Inter` |
| `FontFamilyCJK` | string | `Noto Sans SC` |
| `AccentFill` | color | `mode=light: #202020 / mode=dark: #F5F5F5` |
| `AccentSoft` | color | `mode=light: #E5E5E5 / mode=dark: #3A3A3A` |
| `AccentOnFill` | color | `mode=light: #FFFFFF / mode=dark: #202020` |
| `ColorOverlay` | color | `mode=light: #00000033 / mode=dark: #00000066` |
| `SizeWindowMinHeight` | number | `176` |
| `SizeWindowHeightAbout` | number | `316` |
| `SizeDialogWidth` | number | `440` |
| `SizeLanguageSelectorWidth` | number | `318` |

## 尺寸与文案

R02 尺寸保持：窗口宽 720，侧栏 144，内容最大宽 528；通用高 176、关于高 316，按内容与上下留白取高后按 Space4 向上取整。提示框宽 440，三段语言选择宽 318。

CLI 0.3.13 不保留 width/height 变量绑定，Size Token 解析为几何数值；颜色、圆角、字号、字重、行高与 padding/gap 保留引用。40 个语言资源键与 Copy 变量作映射，显示文案填入实例，language 主题切换字体。
