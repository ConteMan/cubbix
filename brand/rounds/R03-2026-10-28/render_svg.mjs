// Render owned SVG jobs without a browser or application export pipeline.
import { createRequire } from 'node:module';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { dirname, resolve, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
const require = createRequire(import.meta.url);
const { Resvg } = require('@resvg/resvg-js'); // Pinned dependency: 2.6.2.
const root = dirname(fileURLToPath(import.meta.url));
let input = '';
for await (const chunk of process.stdin) input += chunk;
for (const job of JSON.parse(input)) {
  if (job.file === null) {
    const result = new Resvg(job.svg, { fitTo: { mode: 'width', value: job.width } }).render();
    process.stdout.write(result.asPng());
    continue;
  }
  const target = resolve(root, job.file);
  if (relative(root, target).startsWith('..')) throw new Error('Output must stay in this round');
  await mkdir(dirname(target), { recursive: true });
  const result = new Resvg(job.svg, { fitTo: { mode: 'width', value: job.width } }).render();
  await writeFile(target, result.asPng());
}
