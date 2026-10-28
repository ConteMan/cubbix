# Token 与 Go 标识符映射

本轮提出 65 个样式 Token。名称使用合法的 Go 导出标识符，未来实现使用同名字段；值以 `cubbix.pen` 为设计事实源，`tokens.json` 是可审计快照。

颜色使用 `mode=light/dark`，字体使用 `language=zh-CN/en`。文案事实源是 `locales/`；画布内另保留 35 个 `Copy…` 字符串变量作资源映射，实际显示文案由资源值写入实例。

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
| `ColorAccent` | color | `mode=light: #0068D9 / mode=dark: #5AA8FF` |
| `ColorAccentSoft` | color | `mode=light: #DAE9FC / mode=dark: #263E59` |
| `ColorOnAccent` | color | `mode=light: #FFFFFF / mode=dark: #102640` |
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
| `SizeSidebarWidth` | number | `168` |
| `SizeWindowWidth` | number | `840` |
| `SizeWindowHeight` | number | `580` |
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

## 几何尺寸

`Size…` Token 描述几何尺寸。CLI 0.3.13 未保留 `width/height` 的变量绑定，本轮将其解析为数值尺寸；`padding/gap` 等布局间距仍使用变量。画布位置和几何尺寸不计入间距写死数，统计口径在 `scripts/audit.py` 中固定。

窗口 840 × 580，侧栏宽 168；右侧内容最大宽 528，并在右侧区域居中。设置行高 48。字号、字重、行高、颜色、圆角、padding 和 gap 均保留变量引用。
