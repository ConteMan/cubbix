import { MAX_RESULT } from './limits.js';

// This file is bundled into a blob Worker. No bridge or DOM is imported.
self.onmessage = ({ data: { text, suffix, task } }) => {
  let value;
  try { value = JSON.parse(text); }
  catch (error) {
    self.postMessage({ kind: 'invalid', message: String(error.message) });
    return;
  }
  try {
    if (task === 'format' || task === 'minify') {
      const output = JSON.stringify(value, null, task === 'format' ? 2 : 0);
      self.postMessage({ kind: 'text', text: output });
      return;
    }
    if (!suffix.trim()) { self.postMessage({ kind: 'empty' }); return; }
    const result = new Function('"use strict"; return (\nthis' + suffix + '\n);').call(value);
    if (result === undefined) { self.postMessage({ kind: 'undefined', text: 'undefined' }); return; }
    const kind = Array.isArray(result) ? 'array' : result === null ? 'null' : typeof result;
    if (!['array', 'object', 'number', 'string', 'boolean', 'null'].includes(kind)) {
      self.postMessage({ kind: 'error', reason: 'unsupported' });
      return;
    }
    const output = typeof result === 'number' ? String(result) : JSON.stringify(result, null, 2);
    if (new TextEncoder().encode(output).byteLength > MAX_RESULT) {
      self.postMessage({ kind: 'error', reason: 'resultLimit' });
      return;
    }
    self.postMessage({ kind, text: output });
  } catch (error) {
    self.postMessage({ kind: 'error', message: String(error.message) });
  }
};
