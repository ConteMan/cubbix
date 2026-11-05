// WebView counterpart of keycap.go; both use the same semantic tokens.
// Localized labels retain their complete shortcut for assistive technology.
export function renderShortcutTooltip(tip, label) {
  const shortcut = /^(.*) \(([⌃⌥⇧⌘]+)([^()]+)\)$/u.exec(label);
  tip.replaceChildren();
  tip.setAttribute('aria-label', label);
  if (!shortcut) {
    tip.textContent = label;
    return;
  }
  const text = document.createElement('span');
  text.textContent = shortcut[1];
  const keys = document.createElement('span');
  keys.className = 'keycaps';
  keys.setAttribute('aria-hidden', 'true');
  for (const key of [...shortcut[2], shortcut[3]]) {
    const cap = document.createElement('kbd');
    cap.className = 'keycap';
    cap.textContent = key;
    keys.append(cap);
  }
  tip.append(text, keys);
}
