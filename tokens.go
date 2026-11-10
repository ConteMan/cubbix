package main

type tokenValues struct {
	ColorWindow                         string
	ColorSidebar                        string
	ColorCard                           string
	ColorControl                        string
	ColorControlHover                   string
	ColorPaletteSearch                  string
	ColorText                           string
	ColorTextSecondary                  string
	ColorTextDisabled                   string
	ColorSeparator                      string
	ColorAccent                         string
	ColorAccentSoft                     string
	ColorOnAccent                       string
	ColorThumb                          string
	ColorSuccess                        string
	ColorDanger                         string
	ColorTooltip                        string
	ColorTooltipText                    string
	ColorTransparent                    string
	FontSizeCaption                     float32
	FontSizeSmall                       float32
	FontSizeBody                        float32
	FontSizeTitle                       float32
	FontSizeDisplay                     float32
	LineHeightBody                      float32
	LineHeightTitle                     float32
	RadiusWindow                        float32
	RadiusCard                          float32
	RadiusControl                       float32
	Space0                              float32
	Space2                              float32
	Space4                              float32
	Space6                              float32
	Space8                              float32
	Space10                             float32
	Space12                             float32
	Space16                             float32
	Space20                             float32
	Space24                             float32
	Space32                             float32
	Space40                             float32
	StrokeHairline                      float32
	StrokeIcon                          float32
	SizeIcon                            float32
	SizeIconLarge                       float32
	SizeIconButton                      float32
	SizeSwitchWidth                     float32
	SizeSwitchHeight                    float32
	SizeSwitchThumb                     float32
	SizeSidebarWidth                    float32
	SizeWindowWidth                     float32
	SizeWindowHeight                    float32
	SizeContentMaxWidth                 float32
	SizeRowHeight                       float32
	SizeNavHeight                       float32
	SizeSegmentHeight                   float32
	SizeMenuWidth                       float32
	SizeMenuRowHeight                   float32
	SizeButtonHeight                    float32
	SizeBrandMark                       float32
	OpacityDisabled                     float32
	FontWeightRegular                   string
	FontWeightMedium                    string
	FontWeightSemibold                  string
	FontFamily                          string
	FontFamilyCJK                       string
	AccentFill                          string
	AccentSoft                          string
	AccentOnFill                        string
	ColorOverlay                        string
	SizeWindowMinHeight                 float32
	SizeWindowHeightAbout               float32
	SizeDialogWidth                     float32
	SizeLanguageSelectorWidth           float32
	ColorNavHover                       string
	ColorMenuSurface                    string
	ColorSyntaxKey                      string
	ColorSyntaxString                   string
	ColorSyntaxNumber                   string
	ColorSyntaxBool                     string
	ColorSyntaxNull                     string
	ColorSyntaxIndent                   string
	SizeR05MenuWidth                    float32
	SizeR05BoardWidth                   float32
	SizeR05SettingsHeight               float32
	SizeR05RecorderWidth                float32
	SizeR05FieldWidth                   float32
	SizeMenubar18                       float32
	SizeMenubar16                       float32
	FontFamilyCode                      string
	SizeJSONWindowWidth                 float32
	SizeJSONWindowHeight                float32
	SizeJSONEmptyWidth                  float32
	SizeJSONEmptyHeight                 float32
	SizeJSONStateWidth                  float32
	SizeJSONStateHeight                 float32
	SizeJSONToolbarHeight               float32
	SizeJSONTitleInset                  float32
	SizeJSONExpressionHeight            float32
	SizeJSONResultHeight                float32
	SizeJSONLineHeight                  float32
	SizeJSONSplitter                    float32
	SizeJSONPaneMinWidth                float32
	SizeJSONBoardWidth                  float32
	SizeJSONSettingsHeight              float32
	SizePaletteWidth                    float32
	SizePaletteHeight                   float32
	SizePaletteTopOffset                float32
	SizePaletteSearchHeight             float32
	SizePaletteRowHeight                float32
	SizePaletteGroupHeight              float32
	SizeMenubarPanelWidth               float32
	SizePillButtonWidth                 float32
	SizeIconSmall                       float32
	SizeToastSlotWidth                  float32
	SizeToastHeight                     float32
	SizeFeedbackPositionWidth           float32
	ColorPanelStrokeOuter               string
	ColorPanelStrokeInner               string
	ColorPanelShadow                    string
	ColorGuideLine                      string
	ColorGuideLineSnap                  string
	SizePaletteShadowMargin             float32
	SizePaletteSnap                     float32
	ColorKeycap                         string
	ColorKeycapStroke                   string
	ColorKeycapText                     string
	SizeKeycapHeight                    float32
	SizeKeycapMinWidth                  float32
	SizeScrollbar                       float32
	SizePaletteScrollbarOverhang        float32
	SizePalettePlaceholderInset         float32
	SizePageHeader                      float32
	SizeGroupHeader                     float32
	SizeNavItem                         float32
	SizeTranslateWidth                  float32
	SizeTranslateMinHeight              float32
	SizeTranslateWindowHeight           float32
	SizeTranslateMaxHeight              float32
	SizeTranslateInputBase              float32
	SizeTranslateInputLine              float32
	SizeTranslateInputMaxLines          float32
	SizeTranslateResultMax              float32
	SizeTranslateDictionaryPreviewLines float32
	SizeTranslateDictionaryMax          float32
	SizeTranslateDirectionHeight        float32
	SizeQRWidth                         float32
	SizeQRWindowHeight                  float32
	SizeQRMinHeight                     float32
	SizeQRInputHeight                   float32
	SizeQRCodeMax                       float32
}

func tokens(dark bool, language string) tokenValues {
	t := tokenValues{
		ColorWindow:                         "#F5F5F5",
		ColorSidebar:                        "#EBEBEB",
		ColorCard:                           "#FFFFFF",
		ColorControl:                        "#F0F0F0",
		ColorControlHover:                   "#E6E6E6",
		ColorPaletteSearch:                  "#F0F0F0",
		ColorText:                           "#202020",
		ColorTextSecondary:                  "#636363",
		ColorTextDisabled:                   "#858585",
		ColorSeparator:                      "#DEDEDE",
		ColorThumb:                          "#FFFFFF",
		ColorDanger:                         "#BC3030",
		ColorTooltip:                        "#252525",
		ColorTooltipText:                    "#FFFFFF",
		ColorTransparent:                    "#00000000",
		FontSizeCaption:                     11,
		FontSizeSmall:                       12,
		FontSizeBody:                        13,
		FontSizeTitle:                       20,
		FontSizeDisplay:                     28,
		LineHeightBody:                      1.5,
		LineHeightTitle:                     1.3,
		RadiusWindow:                        12,
		RadiusCard:                          8,
		RadiusControl:                       6,
		Space0:                              0,
		Space2:                              2,
		Space4:                              4,
		Space6:                              6,
		Space8:                              8,
		Space10:                             10,
		Space12:                             12,
		Space16:                             16,
		Space20:                             20,
		Space24:                             24,
		Space32:                             32,
		Space40:                             40,
		StrokeHairline:                      1,
		StrokeIcon:                          2,
		SizeIcon:                            16,
		SizeIconLarge:                       24,
		SizeIconButton:                      28,
		SizeSwitchWidth:                     34,
		SizeSwitchHeight:                    20,
		SizeSwitchThumb:                     14,
		SizeSidebarWidth:                    144,
		SizeWindowWidth:                     720,
		SizeWindowHeight:                    176,
		SizeContentMaxWidth:                 528,
		SizeRowHeight:                       48,
		SizeNavHeight:                       32,
		SizeSegmentHeight:                   28,
		SizeMenuWidth:                       260,
		SizeMenuRowHeight:                   32,
		SizeButtonHeight:                    28,
		SizeBrandMark:                       20,
		OpacityDisabled:                     0.45,
		FontWeightRegular:                   "400",
		FontWeightMedium:                    "500",
		FontWeightSemibold:                  "600",
		FontFamily:                          "Noto Sans SC",
		FontFamilyCJK:                       "Noto Sans SC",
		AccentFill:                          "#202020",
		AccentSoft:                          "#E0E0E0",
		AccentOnFill:                        "#FFFFFF",
		ColorOverlay:                        "#00000033",
		SizeWindowMinHeight:                 176,
		SizeWindowHeightAbout:               316,
		SizeDialogWidth:                     440,
		SizeLanguageSelectorWidth:           318,
		ColorNavHover:                       "#F3F3F3",
		ColorMenuSurface:                    "#FFFFFFEB",
		ColorSyntaxKey:                      "#A31515",
		ColorSyntaxString:                   "#0953B7",
		ColorSyntaxNumber:                   "#097335",
		ColorSyntaxBool:                     "#003388",
		ColorSyntaxNull:                     "#003388",
		ColorSyntaxIndent:                   "#EBEBEB",
		SizeR05MenuWidth:                    352,
		SizeR05BoardWidth:                   1520,
		SizeR05SettingsHeight:               232,
		SizeR05RecorderWidth:                188,
		SizeR05FieldWidth:                   164,
		SizeMenubar18:                       18,
		SizeMenubar16:                       16,
		FontFamilyCode:                      "JetBrains Mono",
		SizeJSONWindowWidth:                 840,
		SizeJSONWindowHeight:                520,
		SizeJSONEmptyWidth:                  672,
		SizeJSONEmptyHeight:                 220,
		SizeJSONStateWidth:                  756,
		SizeJSONStateHeight:                 182,
		SizeJSONToolbarHeight:               44,
		SizeJSONTitleInset:                  80,
		SizeJSONExpressionHeight:            44,
		SizeJSONResultHeight:                56,
		SizeJSONLineHeight:                  24,
		SizeJSONSplitter:                    7,
		SizeJSONPaneMinWidth:                240,
		SizeJSONBoardWidth:                  1600,
		SizeJSONSettingsHeight:              340,
		SizePaletteWidth:                    640,
		SizePaletteHeight:                   400,
		SizePaletteTopOffset:                96,
		SizePaletteSearchHeight:             36,
		SizePaletteRowHeight:                36,
		SizePaletteGroupHeight:              24,
		SizeMenubarPanelWidth:               320,
		SizePillButtonWidth:                 64,
		SizeIconSmall:                       12,
		SizeToastSlotWidth:                  320,
		SizeToastHeight:                     36,
		SizeFeedbackPositionWidth:           144,
		ColorPanelStrokeOuter:               "#00000014",
		ColorPanelStrokeInner:               "#FFFFFFB3",
		ColorPanelShadow:                    "#0000002E",
		ColorGuideLine:                      "#2020203D",
		ColorGuideLineSnap:                  "#202020CC",
		SizePaletteShadowMargin:             32,
		SizePaletteSnap:                     8,
		ColorKeycap:                         "#FFFFFF",
		ColorKeycapStroke:                   "#00000024",
		SizeKeycapHeight:                    20,
		SizeKeycapMinWidth:                  20,
		SizeScrollbar:                       6,
		SizePaletteScrollbarOverhang:        9,
		SizePalettePlaceholderInset:         6,
		SizePageHeader:                      56,
		SizeGroupHeader:                     24,
		SizeNavItem:                         40,
		SizeTranslateWidth:                  480,
		SizeTranslateMinHeight:              260,
		SizeTranslateWindowHeight:           360,
		SizeTranslateMaxHeight:              520,
		SizeTranslateInputBase:              36,
		SizeTranslateInputLine:              20,
		SizeTranslateInputMaxLines:          4,
		SizeTranslateResultMax:              320,
		SizeTranslateDictionaryPreviewLines: 3,
		SizeTranslateDictionaryMax:          96,
		SizeTranslateDirectionHeight:        20,
		SizeQRWidth:                         480,
		SizeQRWindowHeight:                  520,
		SizeQRMinHeight:                     440,
		SizeQRInputHeight:                   72,
		SizeQRCodeMax:                       320,
	}
	if dark {
		t.ColorWindow = "#202020"
		t.ColorSidebar = "#292929"
		t.ColorCard = "#2C2C2C"
		t.ColorControl = "#3B3B3B"
		t.ColorControlHover = "#494949"
		t.ColorPaletteSearch = "#464646"
		t.ColorText = "#F5F5F5"
		t.ColorTextSecondary = "#B5B5B5"
		t.ColorTextDisabled = "#929292"
		t.ColorSeparator = "#474747"
		t.ColorThumb = "#202020"
		t.ColorDanger = "#FF8D8D"
		t.ColorTooltip = "#F0F0F0"
		t.ColorTooltipText = "#202020"
		t.ColorTransparent = "#00000000"
		t.AccentFill = "#F5F5F5"
		t.AccentSoft = "#414141"
		t.AccentOnFill = "#202020"
		t.ColorOverlay = "#00000066"
		t.ColorNavHover = "#333333"
		t.ColorMenuSurface = "#2C2C2CEB"
		t.ColorSyntaxKey = "#FF7B72"
		t.ColorSyntaxString = "#79C0FF"
		t.ColorSyntaxNumber = "#7EE787"
		t.ColorSyntaxBool = "#6CB6FF"
		t.ColorSyntaxNull = "#6CB6FF"
		t.ColorSyntaxIndent = "#3C3C3C"
		t.ColorPanelStrokeOuter = "#FFFFFF1F"
		t.ColorPanelStrokeInner = "#FFFFFF14"
		t.ColorPanelShadow = "#0000007A"
		t.ColorGuideLine = "#F5F5F53D"
		t.ColorGuideLineSnap = "#F5F5F5CC"
		t.ColorKeycap = "#454545"
		t.ColorKeycapStroke = "#FFFFFF29"
	}
	if language == "en" {
		t.FontFamily = "Inter"
	}
	t.ColorAccent = t.AccentFill
	t.ColorAccentSoft = t.AccentSoft
	t.ColorOnAccent = t.AccentOnFill
	t.ColorSuccess = t.ColorText
	t.ColorKeycapText = t.ColorTextSecondary
	return t
}
