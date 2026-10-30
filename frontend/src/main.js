import { EditorState, Compartment, Transaction } from '@codemirror/state';
import { EditorView, lineNumbers, drawSelection, keymap } from '@codemirror/view';
import { json } from '@codemirror/lang-json';
import { foldGutter, foldAll, unfoldAll, syntaxHighlighting, HighlightStyle } from '@codemirror/language';
import { history, defaultKeymap, historyKeymap, indentWithTab } from '@codemirror/commands';
import { tags } from '@lezer/highlight';
import { largeDocument, DEBOUNCE_MS } from './limits.js';
import { startJob } from './evaluator.js';

const $ = id => document.getElementById(id);
const bridge = (method, ...args) => window.mygo.call('JSON.' + method, ...args);
const language = new Compartment();
const highlight = HighlightStyle.define([
  { tag: tags.propertyName, color: 'var(--ColorText)', fontWeight: 'var(--FontWeightSemibold)' },
  { tag: tags.number, color: 'var(--ColorText)' },
  { tag: [tags.string, tags.bool, tags.null, tags.punctuation], color: 'var(--ColorTextSecondary)' },
]);
const syntax = () => [json(), foldGutter(), syntaxHighlighting(highlight)];
let config, editor, resultEditor, revision = 0, savedRevision = 0, pinned = false;
let large = false, generation = 0, job, debounce, spinner, result, status = '', detail = '';
let changeQueue = Promise.resolve();
const buttons = {};
const text = key => config.messages[key] ?? key;
const icon = name => ICONS[name];

let tipTimer, tipOwner;
function hideTooltip() {
  clearTimeout(tipTimer);
  tipOwner?.removeAttribute('aria-describedby');
  tipOwner = undefined;
  $('tooltip').hidden = true;
}
function setTooltip(element, label) {
  element.dataset.tooltip = label;
  element.setAttribute('aria-label', label);
  if (element === tipOwner) hideTooltip();
}
function showTooltip(element) {
  hideTooltip();
  if (!element?.dataset.tooltip || element.disabled || element.hidden) return;
  tipTimer = setTimeout(() => {
    tipOwner = element;
    const tip = $('tooltip');
    tip.textContent = element.dataset.tooltip;
    tip.hidden = false;
    element.setAttribute('aria-describedby', 'tooltip');
    const bounds = element.getBoundingClientRect();
    const gap = config.light.Space8;
    tip.style.left = Math.max(gap, Math.min(innerWidth - tip.offsetWidth - gap, bounds.right - tip.offsetWidth)) + 'px';
    tip.style.top = (bounds.bottom + gap + tip.offsetHeight < innerHeight ? bounds.bottom + gap : bounds.top - tip.offsetHeight - gap) + 'px';
  }, 450);
}
function button(parent, id, glyph, label, action) {
  const element = document.createElement('button');
  element.type = 'button';
  element.id = id;
  element.innerHTML = icon(glyph);
  setTooltip(element, text(label));
  element.addEventListener('click', () => { void action(); });
  $(parent).append(element);
  buttons[id] = element;
  return element;
}

function applyConfig(next) {
  config = next;
  document.documentElement.lang = config.locale;
  const values = matchMedia('(prefers-color-scheme: dark)').matches ? config.dark : config.light;
  for (const [key, value] of Object.entries(values)) {
    document.documentElement.style.setProperty('--' + key,
      typeof value === 'number' && !key.startsWith('Opacity') && !key.startsWith('LineHeight') ? value + 'px' : String(value));
  }
  $('suffix').setAttribute('aria-label', text('JSONExpression'));
  $('splitter').setAttribute('aria-label', text('JSONResultResize'));
  const labels = { pin: pinned ? 'R07Unpin' : 'R07Pin', open: 'R07Open', copy: 'R07Copy', format: 'R07Format', fold: 'R07Fold', unfold: 'R07Unfold', minify: 'R07Minify' };
  for (const [id, key] of Object.entries(labels)) {
    if (buttons[id]) {
      setTooltip(buttons[id], text(key));
    }
  }
  if ($('gauge')) setTooltip($('gauge'), text('JSONLargeTip'));
  paintStatus();
}

function paintStatus() {
  const element = $('status');
  element.className = status === 'working' ? '' : 'error';
  element.innerHTML = status ? icon(status === 'working' ? 'loader-circle' : 'triangle-alert') : '';
  if (status === 'working') element.firstElementChild.classList.add('loader');
  const key = { working: 'JSONWorking', invalid: 'JSONInvalid', timeout: 'R07TimeoutTip', error: 'JSONEvaluationError', read: 'JSONReadError', copy: 'JSONCopyError', save: 'JSONWriteError', fileLimit: 'JSONFileLimit', resultLimit: 'JSONResultLimit', unsupported: 'JSONUnsupported', worker: 'JSONWorkerError' }[status];
  const label = key ? text(key) + (detail ? ': ' + detail : '') : '';
  setTooltip(element, label);
}

function setStatus(kind = '', message = '') { status = kind; detail = message; paintStatus(); }
function clearResult() {
  resultEditor?.destroy(); resultEditor = undefined;
  result = undefined;
  $('result-content').replaceChildren();
  $('result').hidden = true;
}
function renderResult(value) {
  clearResult();
  result = value;
  if (['empty', 'invalid', 'error', 'timeout', 'canceled'].includes(value.kind)) return;
  const structured = value.kind === 'array' || value.kind === 'object';
  $('result').hidden = false;
  $('result').classList.toggle('structured', structured);
  if (structured) {
    // A bounded, read-only CodeMirror shares folding behavior with the editor.
    resultEditor = new EditorView({ parent: $('result-content'), state: EditorState.create({
      doc: value.text,
      extensions: [syntax(), EditorState.readOnly.of(true), EditorView.editable.of(false), EditorView.theme({ '&': { background: 'transparent' }, '.cm-scroller': { overflow: 'visible' }, '.cm-content': { padding: '0 !important' } })],
    }) });
  } else {
    const pre = document.createElement('pre');
    pre.textContent = value.text;
    if (value.kind === 'undefined') pre.className = 'muted';
    $('result-content').append(pre);
  }
}

function cancelJob() {
  hideTooltip();
  generation++;
  clearTimeout(debounce); debounce = undefined;
  clearTimeout(spinner);
  job?.cancel(); job = undefined;
  clearResult();
  setStatus();
}
async function run(task = 'evaluate', expectedRevision = revision) {
  const token = ++generation;
  job?.cancel();
  const source = editor.state.doc.toString();
  if (!source.trim()) { clearResult(); setStatus(); return { kind: 'empty' }; }
  job = startJob(WORKER_SOURCE, { text: source, suffix: $('suffix').value, task });
  clearTimeout(spinner);
  spinner = setTimeout(() => { if (token === generation) setStatus('working'); }, 80);
  const answer = await job.promise;
  if (token !== generation || revision !== expectedRevision) return { kind: 'canceled' };
  clearTimeout(spinner);
  job = undefined;
  clearResult();
  if (['invalid', 'error', 'timeout'].includes(answer.kind)) setStatus(answer.reason || answer.kind, answer.message || '');
  else setStatus();
  if (task === 'evaluate') renderResult(answer);
  return answer;
}
function schedule() {
  cancelJob();
  debounce = setTimeout(() => { debounce = undefined; void run(); }, DEBOUNCE_MS);
}
function reportChange() {
  const version = revision;
  const dirty = version !== savedRevision;
  changeQueue = changeQueue.then(() => bridge('Changed', version, dirty)).catch(() => setStatus('save'));
}
function updateControls() {
  const empty = editor.state.doc.length === 0;
  document.body.classList.toggle('empty', empty);
  $('empty').hidden = !empty;
  for (const id of ['copy', 'format', 'minify']) buttons[id].disabled = empty;
  for (const id of ['fold', 'unfold']) buttons[id].disabled = empty || large;
  $('gauge').hidden = !large;
}
function setDocument(value, nextRevision, clean) {
  cancelJob();
  for (const [id, glyph, key] of [['copy', 'copy', 'R07Copy'], ['minify', 'clipboard-copy', 'R07Minify']]) {
    buttons[id].innerHTML = icon(glyph); setTooltip(buttons[id], text(key));
  }
  revision = nextRevision;
  savedRevision = clean ? revision : -1;
  const nextLarge = largeDocument(value);
  editor.setState(makeState(value, nextLarge));
  large = nextLarge;
  updateControls();
  reportChange();
  schedule();
}
function makeState(doc, big) {
  return EditorState.create({ doc, extensions: [
    lineNumbers(), history(), drawSelection(), keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
    language.of(big ? [] : syntax()),
    EditorView.contentAttributes.of({ 'aria-label': 'JSON', spellcheck: 'false' }),
    EditorView.updateListener.of(update => {
      if (!update.docChanged) return;
      revision++;
      const nextLarge = largeDocument(update.state.doc.toString());
      // Reconfiguration is deferred until this update has finished.
      if (nextLarge !== large) {
        large = nextLarge;
        queueMicrotask(() => editor.dispatch({ effects: language.reconfigure(large ? [] : syntax()) }));
      }
      updateControls(); reportChange(); schedule();
    }),
    EditorView.domEventHandlers({ paste(event) {
      // Format a whole-document paste only; ordinary edits stay untouched.
      const selection = editor.state.selection.main;
      if (editor.state.doc.length !== 0 && (selection.from !== 0 || selection.to !== editor.state.doc.length)) return false;
      const value = event.clipboardData?.getData('text/plain');
      if (!value) return false;
      event.preventDefault();
      editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value } });
      if (!large) void format();
      return true;
    } }),
  ] });
}
async function format() {
  cancelJob();
  const version = revision;
  const answer = await run('format', version);
  if (answer?.kind === 'text' && answer.text !== editor.state.doc.toString()) {
    editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: answer.text }, annotations: Transaction.userEvent.of('input.format') });
  } else if (answer?.kind === 'text') schedule();
}
async function copy(minify = false) {
  const version = revision;
  let value = editor.state.doc.toString();
  if (minify) {
    cancelJob();
    const answer = await run('minify', version);
    if (answer?.kind !== 'text') return;
    value = answer.text;
  }
  try {
    await bridge('Copy', value);
    const target = buttons[minify ? 'minify' : 'copy'];
    target.innerHTML = icon('check'); setTooltip(target, text('R07CopySuccess'));
    setTimeout(() => { target.innerHTML = icon(minify ? 'clipboard-copy' : 'copy'); setTooltip(target, text(minify ? 'R07Minify' : 'R07Copy')); }, 1200);
  } catch { setStatus('copy'); }
}
async function save(as = false) {
  const version = revision;
  try {
    await changeQueue;
    const ok = await bridge('Save', editor.state.doc.toString(), version, as);
    if (ok) { savedRevision = version; setStatus(); reportChange(); }
    return ok;
  } catch { setStatus('save'); return false; }
}
async function open() {
  try { await bridge('Open'); } catch { setStatus('read'); }
}
async function pin() {
  pinned = await bridge('Pin');
  buttons.pin.setAttribute('aria-pressed', String(pinned));
  applyConfig(config);
}

async function boot() {
  config = await bridge('Boot');
  revision = config.revision;
  savedRevision = config.clean ? revision : -1;
  pinned = config.pinned;
  $('empty').innerHTML = icon('file-input');
  button('top-actions', 'pin', 'pin', 'R07Pin', pin).setAttribute('aria-pressed', String(pinned));
  button('top-actions', 'open', 'folder-open', 'R07Open', open);
  button('top-actions', 'copy', 'copy', 'R07Copy', () => copy());
  button('tools', 'format', 'braces', 'R07Format', format);
  button('tools', 'fold', 'fold-vertical', 'R07Fold', () => foldAll(editor));
  button('tools', 'unfold', 'unfold-vertical', 'R07Unfold', () => unfoldAll(editor));
  button('tools', 'minify', 'clipboard-copy', 'R07Minify', () => copy(true));
  const gauge = document.createElement('span'); gauge.id = 'gauge'; gauge.tabIndex = 0; gauge.innerHTML = icon('gauge'); $('tools').append(gauge);
  applyConfig(config);
  document.addEventListener('pointerover', event => showTooltip(event.target.closest('[data-tooltip]')));
  document.addEventListener('pointerout', hideTooltip);
  document.addEventListener('focusin', event => { if (event.target.matches(':focus-visible')) showTooltip(event.target.closest('[data-tooltip]')); });
  document.addEventListener('focusout', hideTooltip);
  document.addEventListener('pointerdown', hideTooltip);
  document.addEventListener('keydown', event => { if (event.key === 'Escape') hideTooltip(); });
  large = largeDocument(config.text);
  editor = new EditorView({ parent: $('editor'), state: makeState(config.text, large) });
  // The empty-state icon must paint over the editor.
  $('editor').append($('empty'));
  updateControls();
  $('suffix').addEventListener('input', schedule);
  document.addEventListener('keydown', event => {
    if (event.isComposing || !event.metaKey) return;
    const key = event.key.toLowerCase();
    let action;
    if (event.altKey && key === 'p') action = pin;
    else if (!event.altKey && !event.ctrlKey && key === 'o') action = open;
    else if (!event.altKey && !event.ctrlKey && key === 's') action = () => save(event.shiftKey);
    else if (!event.altKey && !event.ctrlKey && event.shiftKey && key === 'c') action = () => copy();
    else if (!event.altKey && !event.ctrlKey && key === 'w') action = () => bridge('Close');
    if (action) { event.preventDefault(); event.stopPropagation(); void action(); }
  }, true);
  let resizeStart;
  $('splitter').addEventListener('pointerdown', event => {
    if (!$('result').classList.contains('structured')) return;
    resizeStart = { y: event.clientY, height: $('result').offsetHeight };
    $('splitter').setPointerCapture(event.pointerId);
  });
  $('splitter').addEventListener('pointermove', event => {
    if (!resizeStart) return;
    const minimum = config.light.SizeJSONResultHeight;
    $('result').style.setProperty('--result-height', Math.max(minimum, Math.min(innerHeight * .45, resizeStart.height + resizeStart.y - event.clientY)) + 'px');
  });
  $('splitter').addEventListener('pointerup', () => { resizeStart = undefined; });
  $('splitter').addEventListener('pointercancel', () => { resizeStart = undefined; });
  $('splitter').addEventListener('keydown', event => {
    if (!['ArrowUp', 'ArrowDown'].includes(event.key) || !$('result').classList.contains('structured')) return;
    event.preventDefault();
    $('result').style.setProperty('--result-height', Math.max(config.light.SizeJSONResultHeight, Math.min(innerHeight * .45, $('result').offsetHeight + (event.key === 'ArrowUp' ? config.light.Space16 : -config.light.Space16))) + 'px');
  });
  window.mygo.on('mygo:locale', async () => applyConfig(await bridge('Appearance')));
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => applyConfig(config));
  window.addEventListener('pagehide', () => { cancelJob(); editor.destroy(); resultEditor?.destroy(); });
  window.cubbix = {
    lock: value => { document.body.inert = value; if (value) cancelJob(); else if (status !== 'save') schedule(); },
    snapshot: () => ({ text: editor.state.doc.toString(), revision, dirty: revision !== savedRevision }),
    load: (value, nextRevision, clean) => {
      setDocument(value, nextRevision, clean);
      if (!large && value.trim()) void format();
    },
    save, open, pin, error: kind => setStatus(kind),
    dispose: () => { cancelJob(); editor.destroy(); },
    // State inspection and direct transactions are used by hidden WebKit checks.
    editor: () => editor,
    state: () => ({ revision, large, status, result, pinned, generation, pending: !!job || debounce !== undefined }),
    evaluate: async suffix => { $('suffix').value = suffix; cancelJob(); return run(); },
  };
  if (!large && config.text.trim()) void format();
  else schedule();
}
void boot();
