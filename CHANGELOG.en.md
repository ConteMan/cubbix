# Changelog

## 0.5.3 — 2026-11-04

- Fixed the Lid switch staying off after you entered your admin password the first time. If the helper still does not start, the card now tells you, and one click tries again.

## 0.5.2 — 2026-11-04

- Fixed Quit and Restart to Update doing nothing, and Settings not opening, after you had used the command palette.
- The Lid switch now shows up on the card even when your MacBook is closed and running on an external display.

## 0.5.1 — 2026-11-04

- Fixed "Keep awake with the lid closed" always saying development builds aren't supported in the released app. The Lid switch on the card works now.

## 0.5.0 — 2026-11-04

- You can drag the command palette now! Grab an empty spot and move it wherever feels right. Guides appear while you drag and the palette snaps to them, so centering takes one move. Hold ⌥ to place it freely. Each display remembers its own spot, and a double-click on an empty area puts it back.
- The command palette has a softer shadow and a thin outline, so it sits on your desktop more naturally.
- Press ⌃⌥⌘P to open or close the menu bar card, so you can check Keep Awake without clicking the cat.
- Meet the `cubbix` command-line tool: turn Keep Awake on or off, check its status, and open JSON windows from the terminal, scripts or agents, for example `cubbix awake on --minutes 90` or `cubbix awake status --json`. Install it in one click under Settings › General, no admin password needed.

## 0.4.0 — 2026-11-03

- Close the lid and your tasks keep running! Turn on the Lid switch when you start Keep Awake, and agents, downloads and builds won't stop when the lid closes. The first time, you'll enter an admin password once, and Cubbix needs to live in the Applications folder.
- Works on battery too. So your Mac won't drain in your bag, Cubbix lets it sleep normally again once the battery drops to 20% (adjustable in Settings) or after 12 hours.
- When time's up, when you stop it, when you quit Cubbix, even if Cubbix quits unexpectedly, sleep comes back on its own. If it ever doesn't, click the hint in the status row to restore it in one step.
- Your Mac runs a bit warmer with the lid closed, so keep that in mind before packing it away.

## 0.3.1 — 2026-10-31

- Minimize and zoom are back on the main window, drawn by the system. Full screen stays off.
- In the sidebar the mark sits on the left and Cubbix on the right, at the same height, with the name a little smaller.
- The JSON card in the menu bar keeps only the plus beside the title for a new window. The shortcut stays in the tooltip.
- Untitled windows are just called JSON, with no automatic numbers. Click the title to rename it, press Return to keep the name, or Esc to cancel. An opened file uses its filename, and a different file follows the new name.
- Until you run a this expression, the preview on the right stays hidden. The input along the bottom always spans the whole window, with a hairline above it. A mistake shows up only as an icon.
- Fold arrows line up with the line numbers, and the indent guides are still there.

## 0.3.0 — 2026-10-31

- Click the cat and a small card pops up. Keep Awake's switch, mode and durations are right there, so tap 1h and you're set. New JSON windows and the ones you have open live here too.
- Press ⌥ . for the command palette. Just like Raycast, type what you want, such as "awake 45m" or "awake until 18:30", and press Return. ⌘ K shows more actions.
- When you toggle Keep Awake with a shortcut, a small hint in a corner of the screen tells you what happened. Pick where it shows up and how, or turn it off, in Settings.
- Raycast and Shortcuts can now drive Cubbix too, for example by opening cubbix://awake?minutes=60.
- Right-click the cat for a short menu: Settings, Restart to update, and Quit.

## 0.2.1 — 2026-10-30

- JSON windows are now split in two: your data on the left, the this result on the right, with a divider you can drag.
- Keys, strings and numbers each get their own color, and guide lines keep each level lined up.
- Every window has a name now: JSON, JSON 2, or the name of the file you opened.
- It's a quick tool, so closing a window just closes it. No more save prompts.

## 0.2.0 — 2026-10-30

- Say hello to the second cubby, JSON. Press ⌃ ⌥ ⌘ J for a new window, and any JSON on your clipboard shows up neatly formatted, with highlighting, line numbers and folding.
- Type a quick expression in the this bar, like .data.items.length, and see the answer right away. Typos and slow expressions won't freeze anything.
- Open a few windows side by side, and press ⌥ ⌘ P to keep one on top.
- Picking a process to follow no longer stalls. The list now shows only apps and programs running in your terminals.

## 0.1.0 — 2026-10-30

- Meet the first cubby, Keep Awake: tell your Mac to stay up, with the screen on or just your tasks running.
- Pick how long: half an hour, a few hours, until a set time, or for as long as you like.
- Follow a process, such as an agent that is still running. When it finishes, Keep Awake turns itself off.
- Press ⌃ ⌥ ⌘ K to toggle it. While it is on, a small dot sits next to the cat.
- If you unplug while Keep Awake is on, Cubbix gives you a heads-up.

## 0.0.3 — 2026-10-29

- What’s new now lives in Settings. Browse by version, then press Esc to go back.
- Make Settings roomier if you like. Cubbix remembers the size for next time.
- Open a window and Cubbix joins the menu bar and Dock. Close it and the little cat goes back to waiting in the menu bar.

## 0.0.2 — 2026-10-29

- Automatic updates are up and running. Cubbix will get the next version ready for you.

## 0.0.1 — 2026-10-29

- Cubbix has moved into the menu bar. Click the cat to find Settings.
- Choose Chinese, English, or your system language, and decide whether Cubbix opens at login.
- New versions download for you and wait until you choose “Restart to update”. You can check for updates yourself, too.
- Visit “What’s new” to see what just landed.
