import assert from 'node:assert/strict';
import test from 'node:test';
import { Worker } from 'node:worker_threads';
import { build } from 'esbuild';
import { startJob } from '../src/evaluator.js';
import { largeDocument, MAX_LINE, MAX_DOCUMENT } from '../src/limits.js';

const output = await build({ entryPoints: [new URL('../src/worker.js', import.meta.url).pathname], bundle: true, write: false, format: 'iife', minify: true });
const source = output.outputFiles[0].text;
let made = 0, terminated = 0;
const factory = () => {
  made++;
  const worker = new Worker(`const {parentPort}=require('node:worker_threads');global.self=global;self.postMessage=value=>parentPort.postMessage(value);${source};parentPort.on('message',data=>self.onmessage({data}));`, { eval: true });
  const wrapper = {
    postMessage: value => worker.postMessage(value),
    terminate: () => { terminated++; void worker.terminate(); },
  };
  worker.on('message', value => wrapper.onmessage?.({ data: value }));
  worker.on('error', () => wrapper.onerror?.());
  return wrapper;
};
const run = (suffix, text = '{"data":{"items":[{"val":"alpha"},{"val":"beta"}]}}', task = 'evaluate') => startJob(source, { text, suffix, task }, factory).promise;

test('this binds parsed JSON; supported results and errors', async () => {
  for (const [suffix, kind, value] of [
    ['.data.items.length', 'number', '2'],
    ['.data.items.map(x => x.val)', 'array', '[\n  "alpha",\n  "beta"\n]'],
    ['.data', 'object'], ['.data.items[0].val', 'string', '"alpha"'],
    ['.missing', 'undefined', 'undefined'], ['.data.items.length > 0', 'boolean', 'true'],
    ['.missing.length', 'error'], ['.???', 'error'], ['', 'empty'],
    [' && null', 'null', 'null'], [' && (1/0)', 'number', 'Infinity'], [' && (0/0)', 'number', 'NaN'], [' && (() => 1)', 'error'],
    [' && typeof window', 'string', '"undefined"'],
    [' && typeof document', 'string', '"undefined"'],
    [' && typeof mygo', 'string', '"undefined"'],
  ]) {
    const result = await run(suffix);
    assert.equal(result.kind, kind, suffix);
    if (value !== undefined) assert.equal(result.text, value, suffix);
  }
  assert.equal((await run('.x', '{broken')).kind, 'invalid');
  assert.equal((await run(' && "x".repeat(1048577)')).reason, 'resultLimit');
  assert.equal((await run(' && "字".repeat(400000)')).reason, 'resultLimit');
  assert.equal((await run(' && (()=>{ const a={}; a.self=a;return a })()')).kind, 'error');
  assert.equal((await run('', '{"x": 1}', 'format')).text, '{\n  "x": 1\n}');
  assert.equal((await run('', '{\n "x": 1\n}', 'minify')).text, '{"x":1}');
  assert.equal(made, terminated, 'Every completed job must terminate its Worker');
});

test('timeout terminates an actual infinite loop; next job is fresh', async () => {
  const start = performance.now();
  assert.equal((await run(' && (() => { while(true){} })()')).kind, 'timeout');
  assert.ok(performance.now() - start < 800);
  assert.equal((await run('.data.items.length')).text, '2');
  const pending = startJob(source, { text: '{}', suffix: ' && (() => {while(true){}})()' }, factory);
  pending.cancel();
  assert.equal((await pending.promise).kind, 'canceled');
  assert.equal(made, terminated);
});

test('large mode preserves 10 MiB multiline and detects line and total limits', () => {
  assert.equal(largeDocument('x'.repeat(MAX_LINE)), false);
  assert.equal(largeDocument('x'.repeat(MAX_LINE + 1)), true);
  assert.equal(largeDocument((' '.repeat(98) + '\n').repeat(106000)), false);
  assert.equal(largeDocument((' '.repeat(98) + '\n').repeat(Math.ceil(MAX_DOCUMENT / 99) + 1)), true);
});
