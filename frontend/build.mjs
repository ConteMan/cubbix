import { build } from 'esbuild';
import { readFile, writeFile, mkdir, readdir, copyFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = path.dirname(fileURLToPath(import.meta.url));
const out = path.resolve(process.argv[2] || path.join(root, 'dist'));
await mkdir(out, { recursive: true });
const icons = {};
for (const name of ['pin', 'folder-open', 'copy', 'check', 'triangle-alert', 'braces', 'fold-vertical', 'unfold-vertical', 'clipboard-copy', 'file-input', 'gauge', 'loader-circle']) {
  let icon;
  try { icon = await readFile(path.join(root, '../design/assets/lucide-0.468.0', name + '.svg'), 'utf8'); }
  catch { icon = await readFile(path.join(root, 'design/assets/lucide-0.468.0', name + '.svg'), 'utf8'); }
  icons[name] = icon;
}
const worker = await build({ entryPoints: [path.join(root, 'src/worker.js')], bundle: true, minify: true, write: false, target: 'safari17', legalComments: 'inline' });
await build({
  entryPoints: [path.join(root, 'src/main.js')], bundle: true, minify: true,
  outfile: path.join(out, 'main.js'), target: 'safari17', legalComments: 'linked',
  define: { WORKER_SOURCE: JSON.stringify(worker.outputFiles[0].text), ICONS: JSON.stringify(icons) },
  loader: { '.svg': 'text' },
});
for (const name of ['index.html', 'style.css']) await copyFile(path.join(root, 'src', name), path.join(out, name));
const licenses = [];
for (const scope of ['@codemirror', '@lezer', '@marijn']) {
  const dir = path.join(root, 'node_modules', scope);
  try {
    for (const name of (await readdir(dir)).sort()) {
      licenses.push(`## ${scope}/${name}\n\n${await readFile(path.join(dir, name, 'LICENSE'), 'utf8')}`);
    }
  } catch (error) { if (error.code !== 'ENOENT') throw error; }
}
for (const name of ['style-mod', 'w3c-keyname', 'crelt']) {
  try { licenses.push(`## ${name}\n\n${await readFile(path.join(root, 'node_modules', name, 'LICENSE'), 'utf8')}`); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}
let lucide;
try { lucide = await readFile(path.join(root, '../design/assets/lucide-0.468.0/LICENSE'), 'utf8'); } catch { lucide = await readFile(path.join(root, 'design/assets/lucide-0.468.0/LICENSE'), 'utf8'); }
licenses.push('## Lucide 0.468.0\n\n' + lucide);
await writeFile(path.join(out, 'LICENSES.txt'), licenses.join('\n'));
