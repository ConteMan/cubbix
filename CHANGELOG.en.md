# Changelog

## Unreleased

- New `cubbix translate`: text from an argument, a file or a pipe, printing only the translation; `--json` gives stable fields, `--to` chooses the target language, long translations come back intact, and `cubbix help translate` documents it. Typing "Translate <text>" in the command palette keeps the original capitalization, opens the panel and starts the translation.
- New Translate module: a global shortcut opens a small floating panel; type text, press Return, and read the streamed translation. You can copy it, swap the direction, and configure the endpoint, the API key (kept in the system keychain) and the model in Settings. Models can be fetched from the endpoint or typed by hand, and the connection can be tested. A word auto-speak preference is stored but does not speak yet.
- Translate has its own global shortcut and module switch. Turning the module off releases the shortcut while its settings row stays visible, disabled.
- A saved translation key shows only Saved, Replace, and Clear; choose Replace to enter a new one. Keychain operations keep the interface responsive. Multiline streams, timeout and truncation hints, and stale request updates are fixed; switching panels cancels the translation.
- When another tool has already disabled sleep on lid close, the lid switch still works: Cubbix remembers the request, and the helper takes over once the other tool lets go. It never overwrites the other tool's setting and never asks for authorization to do it.
- After taking over, or when the helper's own setting is turned back off, the helper writes it back as soon as the lease allows, with a bounded number of retries. Two tools repeatedly fighting over the setting now stop and show a hint.
- Low battery no longer ends a lid session while plugged in. A new 5% limit is available for tools that calibrate by discharging.
- The helper revision is now 5; the next time you deliberately turn the lid switch on after updating, you will be asked for one administrator authorization.

## 0.6.1 — 2026-11-06

- With more than one display, choose in General where the Settings window opens: follow the pointer, or always on a display you pick. Each display remembers where the window was last time.
- New JSON windows open on the display your pointer is on, and several of them are offset a little instead of stacking on top of each other.
- Unplug a display or change its resolution, and windows move back to where you can see them.

## 0.6.0 — 2026-11-06

- Settings are reorganized: Keep Awake and JSON each get their own page with their settings and shortcuts together, so "Low battery limit" now lives on the Keep Awake page.
- Turning a module off also turns off its shortcuts, so they no longer hold on to system shortcuts, and they come back when you turn it on. If Keep Awake or the lid setting is running, Cubbix lets your Mac sleep normally again before turning it off.
- Every page starts with a short description, hints are clearer, and Automatic updates shows how often it checks and when it last did.
- The command-line tool is now a single switch: turn it on to install it, and you will see a hint if something is wrong.

## 0.5.8 — 2026-11-06

- The command palette looks better and is easier to scan: each command now shows the module it belongs to in small gray text, like "Turn on Keep Awake  Keep Awake".
- Shortcut hints are now separate little keycaps, so ⌘ and K no longer run together.
- The scroll bar no longer covers text, the selected row is balanced on both sides, the line under the search field is gone, and there is a little room between the cursor and the placeholder.
- The search field stands out better in dark mode.

## 0.5.7 — 2026-11-05

- Fixed the lid setting quietly turning off when you switched between keeping the display awake and keeping tasks awake.
- A brief hiccup reading the power setting no longer ends the lid session, and even if the lid setting does fail, Keep Awake itself carries on so your tasks are not affected.
- After updating, you will be asked once to update the helper with your admin password.

## 0.5.6 — 2026-11-05

- Found the real reason the lid helper kept failing to install: the installer looked for a system command in the wrong place. Turn on the Lid switch and enter your admin password once, and it just works now.

## 0.5.5 — 2026-11-04

- Updating the lid helper is steadier: Cubbix now waits for the old helper to quit before starting the new one, so there is nothing to fix by hand.
- If installing the helper ever fails, Cubbix notes which step it stopped at, which makes it easier to sort out.
- The helper log is quieter and only records things that change.

## 0.5.4 — 2026-11-04

- The Lid switch finally turns on! The helper refused to work because of a small folder permission issue, and now it fixes that itself. The first time after updating, you will see "Helper needs an update" and enter your admin password once.
- `cubbix awake on --lid` now waits until the lid setting is really on, and tells you why if it cannot be.

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
