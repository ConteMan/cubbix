#!/usr/bin/env python3
"""Author spec 007's module-grouped settings boards (R16), reopen and export them.

The pinned headless CLI keeps no variable binding in text content and no
variable binding for width and height: copy is written as localized text named
`Copy / <key>`, and geometry is resolved from tokens.json to numbers.

R16 shows the recommended information architecture: one sidebar entry per
module (General / Keep awake / JSON / Shortcuts / About), each page led by a
header card, group titles above the cards, and the three hint levels.
"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R16-2026-11-05-模块分组'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']
BOARDS = [('zh-CN', 'light'), ('zh-CN', 'dark'), ('en', 'light'), ('en', 'dark')]

NUM_KEYS = [
    'Space2', 'Space4', 'Space6', 'Space8', 'Space12', 'Space16', 'Space20', 'Space24',
    'SizeIcon', 'SizeIconButton', 'SizeNavHeight', 'SizeRowHeight', 'SizeSegmentHeight',
    'SizeSwitchWidth', 'SizeSwitchHeight', 'SizeSwitchThumb', 'SizeSidebarWidth',
    'SizeContentMaxWidth', 'SizeR05RecorderWidth', 'SizeLanguageSelectorWidth',
    'SizeFeedbackPositionWidth', 'SizeWindowWidth',
    'RadiusCard', 'RadiusControl', 'RadiusWindow', 'StrokeHairline', 'OpacityDisabled',
    'FontSizeSmall', 'LineHeightBody',
]

# Runtime keys the boards show. Values come from design/locales, so a wording
# change in the resources reaches the boards without touching this script.
LOCALE_KEYS = [
    'General', 'Awake', 'JSON', 'R05Shortcuts', 'About',
    'AwakePageDesc', 'JSONPageDesc', 'ShortcutsPageDesc', 'GeneralPageDesc', 'AboutPageDesc',
    'GroupSettings', 'GroupShortcuts', 'GroupPreferences', 'FeedbackGroup', 'UpdateGroup',
    'R05ShortcutLabel', 'JSONShortcutLabel', 'PaletteShortcutLabel', 'PanelShortcutLabel',
    'FeedbackMode', 'FeedbackHint', 'FeedbackNotification', 'FeedbackOff',
    'FeedbackPosition', 'FeedbackTopLeft', 'FeedbackTopRight', 'FeedbackBottomLeft',
    'FeedbackBottomRight', 'FeedbackCenter',
    'Language', 'System', 'Chinese', 'English', 'LaunchAtLogin', 'LaunchHint',
    'CLIName', 'CLILinkHint', 'LidBatteryLimit', 'LidBatteryHint',
    'Version', 'WhatsNew', 'AutomaticUpdates', 'UpdateSchedule', 'UpdateLastCheck',
    'UpdateToday', 'CheckAction', 'TooltipSuccess',
    'ModuleShortcutOff', 'FeedbackModeInfo', 'FeedbackPositionInfo',
]

COPY = [
    ('R16Title', '设置按模块组织', 'Settings grouped by module'),
    ('R16ModulePagesTitle', '模块页：常亮 / JSON', 'Module pages: keep awake, JSON'),
    ('R16AwakeOn', '常亮开启', 'Keep awake on'),
    ('R16AwakeOff', '常亮关闭：设置收起，快捷键置灰',
     'Keep awake off: settings collapse, shortcut dimmed'),
    ('R16JSONOn', 'JSON 开启', 'JSON on'),
    ('R16JSONOff', 'JSON 关闭：快捷键置灰', 'JSON off: shortcut dimmed'),
    ('R16OtherPagesTitle', '快捷键页与关于页', 'Shortcuts page and About page'),
    ('R16ShortcutsCaption', '快捷键页：Cubbix 与反馈', 'Shortcuts: Cubbix and feedback'),
    ('R16AboutCaption', '关于页：更新', 'About: updates'),
    ('R16GeneralTitle', '通用页：命令行工具', 'General page: command line tool'),
    ('R16CLIOff', '关闭：没有链接', 'Off: no link'),
    ('R16CLIOn', '开启：已建链接', 'On: linked'),
    ('R16CLIConflict', '异常：链接位置被别的文件占用', 'Error: the link path holds another file'),
    ('R16CLIErrorsTitle', '命令行工具的异常状态', 'Command line tool error states'),
    ('R16CLIBroken', '异常：链接失效', 'Error: the link is broken'),
    ('R16CLITranslocated', '异常：应用不在「应用程序」', 'Error: app is outside Applications'),
    ('R16HintTitle', '设置行说明三级', 'Three levels of setting hints'),
    ('R16HintL1', '① 行内小字：一句话、控件窄', '① Inline: one short line, narrow control'),
    ('R16HintL2', '② 第二行：单行省略，行高 48 → 64', '② Second line: one truncated line, row 48 → 64'),
    ('R16HintL3', '③ 提示符号：规则或会变的数据放 tooltip',
     '③ Info icon: rules or changing data go in a tooltip'),
    ('R16HintRule', '判断：不会做错就不写；一句话放 ①；放不下放 ②；两句以上放 ③',
     'Ask whether silence causes mistakes; then ① for one short line, ② when it does not fit, ③ for more'),
    ('R16TooltipLabel', '③ 的 tooltip', 'The ③ tooltip'),
    ('R16MinNote', '窗口最小 720 × 380；内容从顶部排',
     'Minimum window is 720 × 380; content starts at the top'),
]


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Author R16 module settings', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    proc = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                          text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-8000:])
    return proc.stdout


AUTHOR = r'''
const N = __NUM__, S = __STR__;
// Spec 007 raises the minimum window to 720 x 380; every board uses it.
const WINDOW_HEIGHT = 380;
function add(parent, node) {
  const children = node.children || [];
  delete node.children;
  const id = Insert(parent, node);
  for (const child of children) add(id, child);
  return id;
}
function frame(name, more = {}) { return {type: 'frame', name, layout: 'vertical', gap: 0, ...more}; }
function row(name, more = {}) { return frame(name, {layout: 'horizontal', alignItems: 'center', ...more}); }
function text(content, name, more = {}) {
  return {type: 'text', name, content, fontFamily: '$FontFamily', fontSize: '$FontSizeBody',
          fontWeight: '$FontWeightRegular', lineHeight: '$LineHeightBody', fill: '$ColorText', ...more};
}
function copy(language, key, more = {}) {
  return text(S[language][key], 'Copy / ' + key, more);
}
function smallCopy(language, key, more = {}) {
  return copy(language, key, {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary', ...more});
}
function literal(content, more = {}) { return text(content, content, more); }
function spacer() { return frame('Spacer', {width: 'fill_container'}); }
function ic(id, name) {
  return {type: 'ref', ref: id, name: 'Icon / ' + name, width: N.SizeIcon, height: N.SizeIcon};
}
function infoIcon(language, key) {
  return {...ic('uTgqT', 'info'), name: 'Info (tooltip: ' + key + ')'};
}
function alertIcon() {
  return {...ic('bVN9P', 'triangle-alert'), fill: '$ColorDanger',
          name: 'Alert (tooltip: reason)'};
}
function iconTile(id, name, size) {
  const tile = frame('Icon tile / ' + name, {
    width: size || 28, height: size || 28, cornerRadius: '$RadiusControl',
    fill: '$ColorControl', layout: 'horizontal', justifyContent: 'center', alignItems: 'center',
  });
  tile.children = [ic(id, name)];
  return tile;
}
function toggle(on, name, more = {}) {
  const node = {type: 'frame', name, width: N.SizeSwitchWidth, height: N.SizeSwitchHeight,
                layout: 'horizontal', padding: N.Space2, alignItems: 'center',
                justifyContent: on ? 'end' : 'start', cornerRadius: '$RadiusControl',
                fill: on ? '$ColorAccent' : '$ColorControl', gap: 0, ...more};
  if (!on) { node.stroke = '$ColorSeparator'; node.strokeWidth = '$StrokeHairline'; }
  node.children = [frame('Thumb', {width: N.SizeSwitchThumb, height: N.SizeSwitchThumb,
                                   cornerRadius: '$RadiusControl',
                                   fill: on ? '$ColorThumb' : '$ColorTextSecondary'})];
  return node;
}
// Neutral status: solid dot, ColorText when on and ColorTextDisabled when off.
function statusDot(on) {
  return frame('Status dot', {width: N.Space6, height: N.Space6, cornerRadius: N.Space2,
                              fill: on ? '$ColorText' : '$ColorTextDisabled'});
}
function textButton(language, key) {
  const node = frame('Button / ' + key, {
    height: N.SizeIconButton, padding: [0, N.Space12], layout: 'horizontal',
    justifyContent: 'center', alignItems: 'center', cornerRadius: '$RadiusControl',
    stroke: '$ColorSeparator', strokeWidth: '$StrokeHairline',
  });
  node.children = [copy(language, key, {fontSize: '$FontSizeSmall'})];
  return node;
}
function segments(language, entries, selected, width, name, height) {
  const track = row(name, {
    width, height: height || N.SizeSegmentHeight, padding: N.Space2, gap: N.Space2,
    cornerRadius: '$RadiusControl', fill: '$ColorControl', alignItems: 'center',
  });
  track.children = entries.map((entry, index) => {
    const on = index === selected;
    const cell = frame('Segment ' + index, {
      width: 'fill_container', height: 'fill_container', layout: 'vertical',
      justifyContent: 'center', alignItems: 'center', cornerRadius: '$RadiusControl',
      fill: on ? '$ColorAccent' : '$ColorTransparent',
    });
    if (entry.glyph !== undefined) {
      cell.children = [positionGlyph(entry.glyph, on ? '$ColorOnAccent' : '$ColorTextSecondary')];
    } else {
      cell.children = [entry.key
        ? copy(language, entry.key, {fontSize: '$FontSizeSmall', fontWeight: '$FontWeightMedium',
                                     fill: on ? '$ColorOnAccent' : '$ColorText'})
        : literal(entry.literal, {fontSize: '$FontSizeSmall', fontWeight: '$FontWeightMedium',
                                  fill: on ? '$ColorOnAccent' : '$ColorText'})];
    }
    return cell;
  });
  return track;
}
// 16 x 16 screen with one 4 x 4 dot, the view's feedbackPositionGlyph.
function positionGlyph(index, color) {
  const horizontal = index === 4 ? 'center' : (index === 1 || index === 3 ? 'end' : 'start');
  const vertical = index === 4 ? 'center' : (index >= 2 ? 'end' : 'start');
  return frame('Position glyph', {
    width: N.SizeIcon, height: N.SizeIcon, cornerRadius: '$RadiusControl',
    stroke: '$ColorSeparator', strokeWidth: '$StrokeHairline', padding: N.Space2,
    layout: 'horizontal', justifyContent: horizontal, alignItems: vertical,
    children: [frame('Dot', {width: N.Space4, height: N.Space4, cornerRadius: N.Space2, fill: color})],
  });
}
function recorder(symbols, more = {}) {
  const node = row('Shortcut recorder', {
    width: N.SizeR05RecorderWidth, height: N.SizeSegmentHeight, padding: [0, N.Space12],
    gap: N.Space2, cornerRadius: '$RadiusControl', fill: '$ColorControl', ...more});
  node.children = symbols.map(symbol => literal(symbol, {fontSize: '$FontSizeSmall'}));
  return node;
}
function divider() {
  return frame('Divider', {width: 'fill_container', height: N.StrokeHairline, layout: 'vertical',
                           padding: [0, N.Space16],
                           children: [frame('Separator', {width: 'fill_container',
                                                          height: N.StrokeHairline,
                                                          fill: '$ColorSeparator'})]});
}
function card(name, children, more = {}) {
  return frame(name, {width: 'fill_container', layout: 'vertical', gap: 0,
                      cornerRadius: '$RadiusCard', fill: '$ColorCard',
                      stroke: '$ColorSeparator', strokeWidth: '$StrokeHairline', clip: true,
                      ...more, children});
}
// A hint is one line by rule: the frame clips the wrapped text to 18pt.
function singleLine(language, key, more = {}) {
  const clip = frame('Hint / ' + key, {
    width: 'fill_container', height: N.FontSizeSmall * N.LineHeightBody, layout: 'vertical',
  });
  clip.clip = true;
  clip.children = [smallCopy(language, key, {width: 'fill_container',
                                             textGrowth: 'fixed-width', ...more})];
  return clip;
}
// One settings row at the level the spec assigns it.
//   hintKey    -> level ① (inline small copy)
//   lineKey    -> level ② (second line, height 64)
//   infoKey    -> level ③ (info icon beside the title)
function settingRow(language, titleKey, opts = {}) {
  const level2 = Boolean(opts.lineKey);
  const node = row('Row / ' + titleKey, {
    width: 'fill_container', height: level2 ? N.SizeRowHeight + N.Space16 : N.SizeRowHeight,
    padding: [N.Space8, N.Space16], gap: N.Space8,
  });
  const kids = [];
  if (opts.iconId) kids.push(ic(opts.iconId, opts.iconName));
  const title = copy(language, titleKey);
  if (opts.disabled) title.opacity = N.OpacityDisabled;
  if (level2) {
    // Level 2 gives the second line the whole row width: it is a rule, not a
    // caption squeezed between the title and the control.
    const outer = frame('Row / ' + titleKey + ' (level 2)', {
      width: 'fill_container', height: N.SizeRowHeight + N.Space16,
      padding: [N.Space8, N.Space16], gap: N.Space2, justifyContent: 'center',
    });
    const head = row('Row head', {width: 'fill_container', gap: N.Space8});
    const headKids = [];
    if (opts.iconId) headKids.push(ic(opts.iconId, opts.iconName));
    headKids.push(title, spacer());
    if (opts.control) headKids.push(opts.control);
    head.children = headKids;
    const line = row('Hint line', {width: 'fill_container', gap: 0});
    const lineKids = [];
    if (opts.iconId) lineKids.push(frame('Indent', {width: N.SizeIcon + N.Space8}));
    lineKids.push(singleLine(language, opts.lineKey));
    line.children = lineKids;
    outer.children = [head, line];
    return outer;
  } else {
    kids.push(title);
    if (opts.infoKey) kids.push(infoIcon(language, opts.infoKey));
    if (opts.hintKey) kids.push(singleLine(language, opts.hintKey));
    else kids.push(spacer());
  }
  if (opts.alert) kids.push(alertIcon());
  if (opts.statusKey) {
    const status = row('Status', {gap: N.Space6});
    status.children = [statusDot(opts.statusOn),
                       copy(language, opts.statusKey, {fontSize: '$FontSizeSmall',
                                                       fill: '$ColorTextSecondary'})];
    kids.push(status);
  }
  if (opts.chevron) kids.push(ic('MTEP9', 'chevron-right'));
  if (opts.control) kids.push(opts.control);
  node.children = kids;
  return node;
}
function groupTitle(language, key) {
  const head = frame('Group title / ' + key, {
    width: 'fill_container', height: 24, layout: 'horizontal', alignItems: 'center'});
  head.children = [copy(language, key, {fontSize: '$FontSizeSmall',
                                        fontWeight: '$FontWeightMedium',
                                        fill: '$ColorTextSecondary'})];
  return head;
}
function groupTitleLiteral(title) {
  const head = frame('Group title / ' + title, {
    width: 'fill_container', height: 24, layout: 'horizontal', alignItems: 'center'});
  head.children = [literal(title, {fontSize: '$FontSizeSmall', fontWeight: '$FontWeightMedium',
                                   fill: '$ColorTextSecondary'})];
  return head;
}
// The page header card: icon tile, module name, one grey line, page action.
function pageHeader(language, opts) {
  const column = frame('Header text', {width: 'fill_container', gap: N.Space2});
  column.children = [opts.titleLiteral
    ? literal(opts.titleLiteral, {fontWeight: '$FontWeightSemibold'})
    : copy(language, opts.titleKey, {fontWeight: '$FontWeightSemibold'}),
    singleLine(language, opts.descKey)];
  const node = card('Page header', [], {
    height: 56, layout: 'horizontal', alignItems: 'center',
    padding: [N.Space8, N.Space16], gap: N.Space12,
  });
  node.children = [iconTile(opts.iconId, opts.iconName), column];
  if (opts.control) node.children.push(opts.control);
  return node;
}
function shortcutRow(language, titleKey, symbols, disabled) {
  const record = recorder(symbols);
  if (disabled) record.opacity = N.OpacityDisabled;
  const opts = {control: record, disabled};
  if (disabled) opts.infoKey = 'ModuleShortcutOff';
  return settingRow(language, titleKey, opts);
}
function navItem(language, labelKey, iconId, iconName, selected) {
  const node = row('Nav / ' + labelKey, {
    width: 'fill_container', height: 40, padding: [N.Space6, N.Space8], gap: N.Space8,
    cornerRadius: '$RadiusCard', fill: selected ? '$ColorAccentSoft' : '$ColorTransparent',
  });
  node.children = [iconTile(iconId, iconName),
                   copy(language, labelKey, {fontSize: '$FontSizeSmall',
                                             fontWeight: '$FontWeightMedium'})];
  return node;
}
function sidebar(language, page) {
  const nav = frame('Navigation', {width: 'fill_container', layout: 'vertical', gap: N.Space4});
  nav.children = [
    navItem(language, 'General', 'O4KJZC', 'settings-2', page === 0),
    navItem(language, 'Awake', 'lPkvC', 'sun', page === 2),
    navItem(language, 'JSON', 'Hvppf', 'braces', page === 4),
    navItem(language, 'R05Shortcuts', 'H8TPWq', 'keyboard', page === 3),
    navItem(language, 'About', 'uTgqT', 'info', page === 1),
  ];
  return frame('Sidebar', {
    width: N.SizeSidebarWidth, height: 'fill_container', layout: 'vertical', gap: N.Space24,
    padding: [N.Space24, N.Space12], fill: '$ColorSidebar',
    children: [
      frame('Traffic light slot', {width: 'fill_container', height: N.Space12}),
      {type: 'ref', ref: 'Mj9CO', name: 'Brand lockup'},
      nav,
    ],
  });
}
function settingsWindow(language, page, name, content) {
  const main = frame('Main', {
    width: 'fill_container', height: 'fill_container', layout: 'vertical',
    padding: N.Space24, alignItems: 'center',
    children: [frame('Page content', {width: N.SizeContentMaxWidth, layout: 'vertical',
                                      gap: N.Space8, children: content})],
  });
  return frame(name, {
    width: N.SizeWindowWidth, height: WINDOW_HEIGHT, layout: 'horizontal',
    fill: '$ColorWindow', cornerRadius: '$RadiusWindow', clip: true,
    children: [sidebar(language, page), main,
               {type: 'ref', ref: 'aGCdB', name: 'Window controls',
                layoutPosition: 'absolute', x: N.Space12, y: N.Space12}],
  });
}
// --- Pages ---
function awakePage(language, on) {
  const content = [
    pageHeader(language, {
      iconId: 'lPkvC', iconName: 'sun', titleKey: 'Awake', descKey: 'AwakePageDesc',
      control: toggle(on, 'Module switch'),
    }),
  ];
  if (on) {
    content.push(groupTitle(language, 'GroupSettings'));
    content.push(card('Card / settings', [
      settingRow(language, 'LidBatteryLimit', {
        iconId: 'Faq6Y', iconName: 'gauge',
        lineKey: 'LidBatteryHint',
        control: segments(language, [{literal: '10%'}, {literal: '20%'}, {literal: '30%'}], 1,
                          N.SizeLanguageSelectorWidth, 'Battery limit'),
      }),
    ]));
  }
  content.push(groupTitle(language, 'GroupShortcuts'));
  content.push(card('Card / shortcuts', [
    shortcutRow(language, 'R05ShortcutLabel', ['⌃', '⌥', '⌘', 'K'], !on),
  ]));
  return settingsWindow(language, 2, 'Window / ' + language + ' awake ' + (on ? 'on' : 'off'),
                        content);
}
function jsonPage(language, on) {
  return settingsWindow(language, 4, 'Window / ' + language + ' json ' + (on ? 'on' : 'off'), [
    pageHeader(language, {
      iconId: 'Hvppf', iconName: 'braces', titleKey: 'JSON', descKey: 'JSONPageDesc',
      control: toggle(on, 'Module switch'),
    }),
    groupTitle(language, 'GroupShortcuts'),
    card('Card / shortcuts', [
      shortcutRow(language, 'JSONShortcutLabel', ['⌃', '⌥', '⌘', 'J'], !on),
    ]),
  ]);
}
function shortcutsPage(language) {
  const cubbix = card('Card / cubbix', [
    settingRow(language, 'PaletteShortcutLabel', {control: recorder(['⌥', '.'])}),
  ]);
  cubbix.children.push(divider());
  cubbix.children.push(settingRow(language, 'PanelShortcutLabel', {
    control: recorder(['⌃', '⌥', '⌘', 'P'])}));
  const feedback = card('Card / feedback', [
    settingRow(language, 'FeedbackMode', {
      infoKey: 'FeedbackModeInfo',
      control: segments(language,
        [{key: 'FeedbackHint'}, {key: 'FeedbackNotification'}, {key: 'FeedbackOff'}], 0,
        N.SizeLanguageSelectorWidth, 'Feedback mode'),
    }),
  ]);
  feedback.children.push(divider());
  feedback.children.push(settingRow(language, 'FeedbackPosition', {
    infoKey: 'FeedbackPositionInfo',
    control: segments(language,
      [{glyph: 0}, {glyph: 1}, {glyph: 2}, {glyph: 3}, {glyph: 4}], 1,
      N.SizeFeedbackPositionWidth, 'Hint position'),
  }));
  return settingsWindow(language, 3, 'Window / ' + language + ' shortcuts', [
    pageHeader(language, {iconId: 'H8TPWq', iconName: 'keyboard',
                          titleKey: 'R05Shortcuts', descKey: 'ShortcutsPageDesc'}),
    groupTitleLiteral('Cubbix'),
    cubbix,
    groupTitle(language, 'FeedbackGroup'),
    feedback,
  ]);
}
// cliState: 'off' (missing link), 'on' (installed), 'conflict', 'broken', 'translocated'.
function cliRow(language, state) {
  const on = state === 'on' || state === 'broken';
  const control = toggle(on, 'CLI switch');
  if (state === 'translocated') control.opacity = N.OpacityDisabled;
  return settingRow(language, 'CLIName', {
    iconId: 'O8Imme', iconName: 'terminal',
    hintKey: 'CLILinkHint',
    alert: state === 'conflict' || state === 'broken' || state === 'translocated',
    control,
  });
}
function generalPage(language, cliState) {
  const body = card('Card / preferences', [
    settingRow(language, 'Language', {
      control: segments(language, [{key: 'System'}, {key: 'Chinese'}, {key: 'English'}], 1,
                        N.SizeLanguageSelectorWidth, 'Language'),
    }),
  ]);
  body.children.push(divider());
  body.children.push(settingRow(language, 'LaunchAtLogin', {
    iconId: 'vjo7q', iconName: 'power',
    hintKey: 'LaunchHint', control: toggle(true, 'Login switch')}));
  body.children.push(divider());
  body.children.push(cliRow(language, cliState));
  return settingsWindow(language, 0, 'Window / ' + language + ' general ' + cliState, [
    pageHeader(language, {iconId: 'O4KJZC', iconName: 'settings-2',
                          titleKey: 'General', descKey: 'GeneralPageDesc'}),
    groupTitle(language, 'GroupPreferences'),
    body,
  ]);
}
function aboutPage(language) {
  const version = row('Row / Version', {width: 'fill_container', height: N.SizeRowHeight,
                                        padding: [N.Space8, N.Space16], gap: N.Space8});
  const status = row('Status', {gap: N.Space6});
  status.children = [statusDot(true),
                     copy(language, 'TooltipSuccess', {fontSize: '$FontSizeSmall',
                                                       fill: '$ColorTextSecondary'})];
  version.children = [copy(language, 'Version'), spacer(),
                      literal('0.5.7', {fill: '$ColorTextSecondary'}), status];
  const body = card('Card / updates', [version]);
  body.children.push(divider());
  body.children.push(settingRow(language, 'WhatsNew', {chevron: true}));
  body.children.push(divider());
  body.children.push(settingRow(language, 'AutomaticUpdates', {
    iconId: 'YeT0w', iconName: 'refresh-cw',
    lineKey: 'UpdateSchedule', control: toggle(true, 'Updates switch')}));
  return settingsWindow(language, 1, 'Window / ' + language + ' about', [
    pageHeader(language, {iconId: 'uTgqT', iconName: 'info', titleKey: 'About',
                          descKey: 'AboutPageDesc',
                          control: textButton(language, 'CheckAction')}),
    groupTitle(language, 'UpdateGroup'),
    body,
  ]);
}
// The three hint levels, shown as the pattern they come from.
function anatomy(language) {
  const stack = frame('Anatomy', {width: N.SizeContentMaxWidth, layout: 'vertical', gap: N.Space8});
  const l1 = frame('Level 1', {width: 'fill_container', layout: 'vertical', gap: N.Space4});
  l1.children = [smallCopy(language, 'R16HintL1'),
                 card('Card / level 1', [settingRow(language, 'LaunchAtLogin', {
                   iconId: 'vjo7q', iconName: 'power',
                   hintKey: 'LaunchHint', control: toggle(true, 'Login switch')})])];
  const l2 = frame('Level 2', {width: 'fill_container', layout: 'vertical', gap: N.Space4});
  l2.children = [smallCopy(language, 'R16HintL2'),
                 card('Card / level 2', [settingRow(language, 'AutomaticUpdates', {
                   iconId: 'YeT0w', iconName: 'refresh-cw',
                   lineKey: 'UpdateSchedule', control: toggle(true, 'Updates switch')})])];
  const l3 = frame('Level 3', {width: 'fill_container', layout: 'vertical', gap: N.Space4});
  l3.children = [smallCopy(language, 'R16HintL3'),
                 card('Card / level 3', [settingRow(language, 'FeedbackMode', {
                   infoKey: 'FeedbackModeInfo',
                   control: segments(language,
                     [{key: 'FeedbackHint'}, {key: 'FeedbackNotification'}, {key: 'FeedbackOff'}], 0,
                     N.SizeLanguageSelectorWidth, 'Feedback mode')})])];
  stack.children = [l1, l2, l3];
  return stack;
}
function tooltipStage(language) {
  const bubble = frame('Tooltip', {
    width: 320, layout: 'vertical', gap: 0, padding: [N.Space6, N.Space8],
    cornerRadius: '$RadiusControl', fill: '$ColorTooltip',
  });
  bubble.children = [literal(S[language].FeedbackModeInfo,
                             {fontSize: '$FontSizeSmall', fill: '$ColorTooltipText',
                              width: 'fill_container', textGrowth: 'fixed-width'})];
  const stack = frame('Tooltip stage', {width: 352, layout: 'vertical', gap: N.Space8});
  stack.children = [smallCopy(language, 'R16TooltipLabel'), bubble];
  return stack;
}
function withCaption(language, captionKey, node) {
  const column = frame('Caption / ' + captionKey, {
    width: N.SizeWindowWidth, layout: 'vertical', gap: N.Space8,
  });
  column.children = [smallCopy(language, captionKey), node];
  return column;
}
function section(language, titleKey, noteKey, children) {
  const head = row('Section head', {width: 'fill_container', gap: N.Space8});
  head.children = [copy(language, titleKey, {fontWeight: '$FontWeightSemibold'})];
  if (noteKey) head.children.push(smallCopy(language, noteKey,
                                             {width: 'fill_container', textGrowth: 'fixed-width'}));
  const node = frame('Section / ' + titleKey, {width: 2952, layout: 'vertical', gap: N.Space12});
  node.children = [head, row('Windows', {width: 'fill_container', gap: N.Space24,
                                         alignItems: 'start', children})];
  return node;
}
const roots = Get((n, c) => { c.skipChildren(); return {bounds: c.bounds}; });
const y = Math.max(...roots.map(n => n.bounds.y + n.bounds.height)) + 240;
add(document, frame('R16 round label', {
  x: 0, y, width: 840, height: 32,
  children: [copy('zh-CN', 'R16Title', {fontSize: '$FontSizeTitle',
                                        fontWeight: '$FontWeightSemibold'})],
}));
const themes = [['zh-CN', 'light', 0], ['zh-CN', 'dark', 1], ['en', 'light', 2], ['en', 'dark', 3]];
for (const [language, mode, index] of themes) {
  const board = frame('R16 ' + language + ' ' + mode, {
    x: index * 3200, y: y + 112, width: 3000, layout: 'vertical', gap: N.Space24,
    padding: N.Space24, theme: {language, mode}, fill: '$ColorWindow', clip: true,
  });
  board.children = [
    copy(language, 'R16Title', {fontSize: '$FontSizeDisplay',
                                fontWeight: '$FontWeightSemibold'}),
    section(language, 'R16ModulePagesTitle', 'R16MinNote', [
      withCaption(language, 'R16AwakeOn', awakePage(language, true)),
      withCaption(language, 'R16AwakeOff', awakePage(language, false)),
      withCaption(language, 'R16JSONOn', jsonPage(language, true)),
      withCaption(language, 'R16JSONOff', jsonPage(language, false)),
    ]),
    section(language, 'R16OtherPagesTitle', null, [
      withCaption(language, 'R16ShortcutsCaption', shortcutsPage(language)),
      withCaption(language, 'R16AboutCaption', aboutPage(language)),
    ]),
    section(language, 'R16GeneralTitle', null, [
      withCaption(language, 'R16CLIOff', generalPage(language, 'off')),
      withCaption(language, 'R16CLIOn', generalPage(language, 'on')),
      withCaption(language, 'R16CLIConflict', generalPage(language, 'conflict')),
    ]),
    section(language, 'R16CLIErrorsTitle', null, [
      withCaption(language, 'R16CLIBroken', generalPage(language, 'broken')),
      withCaption(language, 'R16CLITranslocated', generalPage(language, 'translocated')),
    ]),
    section(language, 'R16HintTitle', 'R16HintRule', [anatomy(language), tooltipStage(language)]),
  ];
  add(document, board);
}
Print('R16_CREATED', true);
'''

QA = r'''
const boards = Get((n, c) => { c.skipChildren(); return {id: n.id, name: n.name, theme: n.theme}; })
  .filter(n => /^R16 (zh-CN|en) (light|dark)$/.test(n.name));
const qa = {boards: boards.length, nodes: 0, problems: []};
for (const board of boards) {
  Get(board.id, (n, c) => {
    qa.nodes++;
    if (n.type === 'text' && typeof n.content === 'string' && n.content.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Unresolved copy'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.width === 'string' &&
        n.width.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Variable width'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.height === 'string' &&
        n.height.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Variable height'});
    }
    if (c.problems && n.enabled !== false) {
      qa.problems.push({name: n.name, problem: c.problems, bounds: c.bounds});
    }
  }, {resolveInstances: true});
}
Print('R16_QA', qa);
'''


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    tokens = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text())
               for lang in ('zh-CN', 'en')}
    missing = [key for key in LOCALE_KEYS if key not in locales['zh-CN'] or key not in locales['en']]
    if missing:
        raise SystemExit('missing locale keys: ' + ', '.join(missing))
    if not any(n.get('name', '').startswith('R16 ') for n in before['children']):
        variables = {}
        for key in LOCALE_KEYS:
            variables[key] = {'type': 'string', 'value': [
                {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
        for key, zh, en in COPY:
            variables[key] = {'type': 'string', 'value': [
                {'value': zh, 'theme': {'language': 'zh-CN'}},
                {'value': en, 'theme': {'language': 'en'}}]}
        variables = {key: value for key, value in variables.items()
                     if key not in before['variables']}
        strings = {lang: {key: locales[lang][key] for key in LOCALE_KEYS} for lang in locales}
        for lang in strings:
            strings[lang].update({key: zh if lang == 'zh-CN' else en for key, zh, en in COPY})
        numbers = {key: tokens[key]['value'] for key in NUM_KEYS if key in tokens}
        code = 'SetVariables(' + json.dumps(variables, ensure_ascii=False) + ');\n'
        code += AUTHOR.replace('__NUM__', json.dumps(numbers)).replace(
            '__STR__', json.dumps(strings, ensure_ascii=False))
        run(code, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    for node in before['children']:
        assert node == next(n for n in after['children'] if n['id'] == node['id']), node['id']
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': True, 'exports': []}
    with tempfile.TemporaryDirectory(prefix='cubbix-r16-') as tmp:
        tmp = Path(tmp)
        output = run(QA, tmp / 'qa.pen')
        qa = next(json.loads(line[len('R16_QA '):]) for line in output.splitlines()
                  if line.startswith('R16_QA '))
        result['nativeLayout'] = qa
        for node in after['children']:
            if node.get('name') not in [f'R16 {lang} {mode}' for lang, mode in BOARDS]:
                continue
            export_dir = tmp / 'exports'
            run('Export([' + json.dumps(node['id']) + '],"png",' +
                json.dumps(str(export_dir)) + ',{scale:2});', tmp / 'reopen.pen')
            filename = node['name'].replace('R16 ', '').replace(' ', '-') + '.png'
            target = ROUND / filename
            shutil.copyfile(export_dir / (node['id'] + '.png'), target)
            result['exports'].append({'nodeId': node['id'], 'file': filename, 'scale': 2,
                                      'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result['passed'] = len(result['exports']) == 4 and not qa['problems']
    (ROUND / 'render-validation.json').write_text(
        json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
