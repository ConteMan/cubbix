import { BUDGET_MS } from './limits.js';

// A job owns one Worker. Cancellation also resolves its promise, so callers
// can discard obsolete responses without keeping pending tasks or workers.
export function startJob(source, input, makeWorker = url => new Worker(url), budget = BUDGET_MS) {
  const url = URL.createObjectURL(new Blob([source], { type: 'text/javascript' }));
  let worker, timer, finish;
  const promise = new Promise(resolve => {
    let done = false;
    finish = result => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      worker?.terminate();
      URL.revokeObjectURL(url);
      resolve(result);
    };
    timer = setTimeout(() => finish({ kind: 'timeout' }), budget);
    try {
      worker = makeWorker(url);
      worker.onmessage = event => finish(event.data);
      worker.onerror = () => finish({ kind: 'error', reason: 'worker' });
      worker.postMessage(input);
    } catch { finish({ kind: 'error', reason: 'worker' }); }
  });
  return { promise, cancel: () => finish({ kind: 'canceled' }) };
}
