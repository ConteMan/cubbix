#!/usr/bin/env node
// Derive review-only states from the approved raster template, without writing brand/.
import { createRequire } from 'node:module';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const cliFile = process.argv[2];
if (!cliFile) throw new Error('Pass the pinned pen.dev CLI entry point to resolve Sharp.');
const sharp = createRequire(path.resolve(cliFile))('sharp');
const root = fileURLToPath(new URL('../', import.meta.url));
const source = path.join(root, '../brand/dist/menubar-template@2x.png');
const out = path.join(root, 'rounds/R05-2026-10-29-常亮/menubar');
await mkdir(out, { recursive: true });
const original = await readFile(source);
const hash = value => createHash('sha256').update(value).digest('hex');
const { data, info } = await sharp(original).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
if (info.width !== 36 || info.height !== 36) throw new Error('Unexpected template dimensions.');
const closed = Buffer.from(data);
for (let y = 10; y <= 17; y++) for (let x = 9; x <= 26; x++) closed[(y * 36 + x) * 4 + 3] = 255;
const cuts = await sharp(Buffer.from('<svg width="288" height="288" viewBox="0 0 36 36"><path d="M10.5 14h4.5m6 0h4.5" fill="none" stroke="white" stroke-width="1.5" stroke-linecap="round"/></svg>')).resize(36, 36).ensureAlpha().raw().toBuffer();
for (let i = 3; i < closed.length; i += 4) closed[i] = Math.round(closed[i] * (1 - cuts[i] / 255));
const dot = Buffer.from(data);
const halo = await sharp(Buffer.from('<svg width="288" height="288" viewBox="0 0 36 36"><circle cx="31" cy="31" r="3" fill="white"/></svg>')).resize(36, 36).ensureAlpha().raw().toBuffer();
const point = await sharp(Buffer.from('<svg width="288" height="288" viewBox="0 0 36 36"><circle cx="31" cy="31" r="1.8" fill="white"/></svg>')).resize(36, 36).ensureAlpha().raw().toBuffer();
for (let i = 3; i < dot.length; i += 4) dot[i] = Math.max(Math.round(dot[i] * (1 - halo[i] / 255)), point[i]);
let outsideEyeChanges = 0;
for (let y = 0; y < 36; y++) for (let x = 0; x < 36; x++) if (!(x >= 9 && x <= 26 && y >= 10 && y <= 17) && data[(y * 36 + x) * 4 + 3] !== closed[(y * 36 + x) * 4 + 3]) outsideEyeChanges++;
if (outsideEyeChanges) throw new Error('Closed-eye edit changed the silhouette.');
const files = [];
for (const [state, rgba] of [['open', data], ['closed', closed], ['dot', dot]]) {
 for (const pt of [18, 16]) for (const scale of [1, 2]) {
  const size = pt * scale;
  const alpha = await sharp(rgba, { raw: info }).extractChannel(3).resize(size, size).toBuffer();
  for (const theme of ['light', 'dark']) {
   const ink = theme === 'light' ? '#202020' : '#F5F5F5';
   const image = await sharp({ create: { width: size, height: size, channels: 3, background: ink } }).joinChannel(alpha, { raw: { width: size, height: size, channels: 1 } }).png().toBuffer();
   const file = `${state}-${pt}pt-${theme}${scale === 2 ? '@2x' : ''}.png`;
   await writeFile(path.join(out, file), image);
   files.push({ file, state, pt, scale, sha256: hash(image) });
   const preview = await sharp(image).flatten({ background: theme === 'light' ? '#EBEBEB' : '#292929' }).png().toBuffer();
   const previewFile = file.replace('.png', '-preview.png');
   await writeFile(path.join(out, previewFile), preview);
   files.push({ file: previewFile, state, pt, scale, background: theme, sha256: hash(preview) });
  }
 }
}
if (hash(await readFile(source)) !== hash(original)) throw new Error('Source changed.');
await writeFile(path.join(out, 'manifest.json'), JSON.stringify({ source: 'brand/dist/menubar-template@2x.png', sourceSha256: hash(original), outsideEyeChanges, eyeEditBounds: [9, 10, 26, 17], files }, null, 2) + '\n');
console.log(`Exported ${files.length} review assets; silhouette changes outside eyes: ${outsideEyeChanges}.`);
