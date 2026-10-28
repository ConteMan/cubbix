import { createRequire } from 'node:module';
import { readdir, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';

const [cliFile, directory] = process.argv.slice(2);
if (!cliFile || !directory) throw new Error('Usage: node palette.mjs <CLI entry file> <PNG directory>');
const sharp = createRequire(resolve(cliFile))('sharp');
const results = [];
for (const filename of (await readdir(directory)).filter(name => name.endsWith('.png')).sort()) {
  const { data, info } = await sharp(join(directory, filename)).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  let invalid = 0;
  let semanticRed = 0;
  for (let offset = 0; offset < data.length; offset += info.channels) {
    const red = data[offset], green = data[offset + 1], blue = data[offset + 2], alpha = data[offset + 3];
    if (!alpha || Math.max(red, green, blue) - Math.min(red, green, blue) <= 2) continue;
    if (red > green && Math.abs(green - blue) <= 2) semanticRed++;
    else invalid++;
  }
  results.push({ file: filename, width: info.width, height: info.height, semanticRedPixels: semanticRed, unexpectedColorPixels: invalid });
}
const output = { scope: 'Native PNG pixels; only neutral RGB or semantic red allowed, with a two-level raster rounding tolerance.', images: results, passed: results.length === 14 && results.every(item => item.unexpectedColorPixels === 0) };
await writeFile(join(directory, 'pixel-validation.json'), JSON.stringify(output, null, 2) + '\n');
console.log(JSON.stringify({ images: results.length, passed: output.passed }));
if (!output.passed) process.exitCode = 1;
