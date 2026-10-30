// UTF-16 code units: the editor and Worker use the same document model.
export const MAX_LINE = 100_000;
export const MAX_DOCUMENT = 32 * 1024 * 1024;
export const MAX_RESULT = 1024 * 1024;
export const BUDGET_MS = 200;
export const DEBOUNCE_MS = 120;

export function largeDocument(text) {
  if (text.length > MAX_DOCUMENT) return true;
  let start = 0;
  for (let i = 0; i < text.length; i++) {
    if (text.charCodeAt(i) === 10) start = i + 1;
    else if (i - start + 1 > MAX_LINE) return true;
  }
  return false;
}
