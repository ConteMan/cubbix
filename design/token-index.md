# Token 与 Go 标识符映射

R02 共 73 个样式 Token。名称使用合法的 Go 导出标识符，未来实现使用同名字段；`cubbix.pen` 是设计事实源，`tokens.json` 是可审计快照。

## accent 组

`groups.accent` 集中维护 `AccentFill`、`AccentSoft`、`AccentOnFill`。旧语义名 `ColorAccent`、`ColorAccentSoft`、`ColorOnAccent` 只作引用别名，画布节点继续使用这些语义名。

`accent=blue` 是当前默认槽，`accent=orange` 仅作对比。Logo 定色后只改该组默认槽的三个值，不改节点。每个槽都按 `mode=light/dark` 定义。

## 全量映射

| Token / Go 字段 | 类型 | 值 |
| --- | --- | --- |
| `ColorWindow` | color | `mode=light: #F5F5F7 / mode=dark: #202022` |
| `ColorSidebar` | color | `mode=light: #EBEBEE / mode=dark: #29292C` |
| `ColorCard` | color | `mode=light: #FFFFFF / mode=dark: #2C2C2F` |
| `ColorControl` | color | `mode=light: #F0F0F3 / mode=dark: #3B3B3F` |
| `ColorControlHover` | color | `mode=light: #E6E6EB / mode=dark: #49494E` |
| `ColorText` | color | `mode=light: #202124 / mode=dark: #F5F5F7` |
| `ColorTextSecondary` | color | `mode=light: #63636B / mode=dark: #B5B5BD` |
| `ColorTextDisabled` | color | `mode=light: #85858F / mode=dark: #92929B` |
| `ColorSeparator` | color | `mode=light: #DEDEE3 / mode=dark: #47474D` |
| `ColorAccent` | color | `$AccentFill` |
| `ColorAccentSoft` | color | `$AccentSoft` |
| `ColorOnAccent` | color | `$AccentOnFill` |
| `ColorThumb` | color | `mode=light: #FFFFFF / mode=dark: #FFFFFF` |
| `ColorSuccess` | color | `mode=light: #18733D / mode=dark: #6ED59A` |
| `ColorDanger` | color | `mode=light: #BC3030 / mode=dark: #FF8D8D` |
| `ColorTooltip` | color | `mode=light: #25252A / mode=dark: #F0F0F4` |
| `ColorTooltipText` | color | `mode=light: #FFFFFF / mode=dark: #202124` |
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
| `AccentFill` | color | `mode=light, accent=blue: #0068D9 / mode=dark, accent=blue: #5AA8FF / mode=light, accent=orange: #B84A00 / mode=dark, accent=orange: #FF9A52` |
| `AccentSoft` | color | `mode=light, accent=blue: #DAE9FC / mode=dark, accent=blue: #263E59 / mode=light, accent=orange: #FFF0E2 / mode=dark, accent=orange: #4E3423` |
| `AccentOnFill` | color | `mode=light, accent=blue: #FFFFFF / mode=dark, accent=blue: #102640 / mode=light, accent=orange: #FFFFFF / mode=dark, accent=orange: #301800` |
| `ColorOverlay` | color | `mode=light: #00000033 / mode=dark: #00000066` |
| `SizeWindowMinHeight` | number | `176` |
| `SizeWindowHeightAbout` | number | `316` |
| `SizeDialogWidth` | number | `440` |
| `SizeLanguageSelectorWidth` | number | `318` |

## 尺寸与文案

固定窗口宽 720、侧栏宽 144、最大内容宽 528；最小高 176，关于页高 316。按内容与上下留白计算高度，再按 Space4 向上取整。提示框宽 440；三段语言选择宽 318。

`Size…` 为几何尺寸。CLI 0.3.13 不保留 width/height 的变量绑定，本轮按 Token 解析数值尺寸；padding/gap 等布局间距仍保留变量。

颜色、字号、字重、行高和圆角均引用变量。语言资源包含 40 个键，画布另保留 40 个 Copy 字符串变量作映射；实际显示文案由资源值写入实例，language 主题切换字体。
