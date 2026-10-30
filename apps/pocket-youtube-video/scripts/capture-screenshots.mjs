#!/usr/bin/env node
/**
 * Capture the Pocket YouTube Video screenshot matrix from a running stack.
 *
 *   POCKET_YOUTUBE_CAPTURE_URL=http://127.0.0.1:5202 \
 *   node scripts/capture-screenshots.mjs [--verify-only]
 *
 * Every capture is a real navigation to the live Vite app backed by videod.
 * The run aborts on a PNG size mismatch, a duplicate SHA-256, horizontal
 * overflow, a short/blank document, or a page heading that did not render.
 * Writes docs/screenshots/manifest.json with the timestamp and hashes.
 */
import {createHash} from 'node:crypto';
import {createRequire} from 'node:module';
import {mkdirSync, readFileSync, writeFileSync, statSync} from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const appDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const {chromium} = createRequire(path.join(appDir, 'frontend', 'package.json'))('playwright-core');
const outDir = path.join(appDir, 'docs', 'screenshots');
const BASE = (process.env.POCKET_YOUTUBE_CAPTURE_URL || 'http://127.0.0.1:5202').replace(/\/$/, '');
const CHROME = process.env.CHROME_PATH || '/usr/bin/google-chrome';
const verifyOnly = process.argv.includes('--verify-only');

// name, width, height, theme, lang, profile, scale, hash route, extra query
const SHOTS = [
  ['accounts', 1280, 720, 'dark', 'en', 'desktop', 100, 'accounts'],
  ['channels', 1280, 720, 'dark', 'en', 'desktop', 100, 'channels'],
  ['downloads', 1280, 720, 'dark', 'en', 'desktop', 100, 'downloads'],
  ['dpi-150', 1280, 720, 'dark', 'en', 'desktop', 150, 'home'],
  ['dpi-200', 1280, 720, 'dark', 'en', 'desktop', 200, 'home'],
  ['dpi-300', 1280, 720, 'dark', 'en', 'desktop', 300, 'home'],
  ['history', 1280, 720, 'dark', 'en', 'desktop', 100, 'history'],
  ['home', 1280, 720, 'dark', 'en', 'desktop', 100, 'home'],
  ['library', 1280, 720, 'dark', 'en', 'desktop', 100, 'library'],
  ['locale-ja', 1280, 720, 'dark', 'ja', 'desktop', 100, 'home'],
  ['locale-zh-CN', 1280, 720, 'dark', 'zh-CN', 'desktop', 100, 'home'],
  ['playlists', 1280, 720, 'dark', 'en', 'desktop', 100, 'playlists'],
  ['profile-dap', 480, 320, 'dark', 'en', 'dap', 100, 'home'],
  ['profile-desktop', 480, 320, 'dark', 'en', 'desktop', 100, 'watch-later'],
  ['profile-handheld', 480, 320, 'dark', 'en', 'handheld', 100, 'home'],
  ['profile-tv', 480, 320, 'dark', 'en', 'tv', 100, 'home'],
  ['search', 1280, 720, 'dark', 'en', 'desktop', 100, 'search', 'q=Demo%20Video%200001'],
  ['settings', 1280, 720, 'dark', 'en', 'desktop', 100, 'settings'],
  ['subscriptions', 1280, 720, 'dark', 'en', 'desktop', 100, 'subscriptions'],
  ['theme-dark', 1280, 720, 'dark', 'en', 'desktop', 100, 'video/video-0002'],
  ['theme-light', 1280, 720, 'light', 'en', 'desktop', 100, 'home'],
  ['theme-oled', 1280, 720, 'amoled', 'en', 'desktop', 100, 'home'],
  ['video-detail', 1280, 720, 'dark', 'en', 'desktop', 100, 'video/video-0001'],
  ['viewport-1280x720', 1280, 720, 'dark', 'en', 'desktop', 100, 'downloads'],
  ['viewport-1920x1080', 1920, 1080, 'dark', 'en', 'desktop', 100, 'home'],
  ['viewport-320x240', 320, 240, 'dark', 'en', 'desktop', 100, 'home'],
  ['viewport-480x320', 480, 320, 'dark', 'en', 'desktop', 100, 'home'],
  ['viewport-640x480', 640, 480, 'dark', 'en', 'desktop', 100, 'home'],
  ['viewport-720x720', 720, 720, 'dark', 'en', 'desktop', 100, 'home'],
  ['watch-later', 1280, 720, 'dark', 'en', 'desktop', 100, 'watch-later'],
];

const url = (s) => {
  const [, , , theme, lang, profile, scale, route, extra] = s;
  const q = new URLSearchParams({theme, lang, profile, scale: String(scale)});
  if (extra) new URLSearchParams(extra).forEach((v, k) => q.set(k, v));
  return `${BASE}/?${q}#${route}`;
};

function pngSize(buf) {
  if (buf.subarray(0, 8).toString('binary') !== '\x89PNG\r\n\x1a\n') throw new Error('not a PNG');
  return [buf.readUInt32BE(16), buf.readUInt32BE(20)];
}

if (!verifyOnly) mkdirSync(outDir, {recursive: true});

const browser = await chromium.launch({
  executablePath: CHROME,
  args: ['--no-sandbox', '--disable-gpu', '--hide-scrollbars'],
});
const context = await browser.newContext({viewport: {width: 1280, height: 720}});
const page = await context.newPage();
const consoleErrors = [];
page.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text()); });
page.on('pageerror', e => consoleErrors.push(`pageerror: ${e.message}`));
page.on('response', r => { if (r.status() >= 400) consoleErrors.push(`http ${r.status()}: ${r.url()}`); });

const entries = [];
const seen = new Map();
for (const shot of SHOTS) {
  const [name, width, height] = shot;
  await page.setViewportSize({width, height});
  await page.goto(url(shot), {waitUntil: 'networkidle', timeout: 45000});
  await page.waitForFunction(() => document.body.innerText.trim().length > 120, {timeout: 20000});
  await page.waitForTimeout(250);
  const audit = await page.evaluate(() => ({
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    text: document.body.innerText.trim().length,
    heading: document.querySelector('h1')?.textContent?.trim() ?? '',
    theme: document.documentElement.dataset.theme ?? '',
    profile: document.documentElement.dataset.profile ?? '',
  }));
  if (audit.overflow > 1) throw new Error(`${name}: horizontal overflow ${audit.scrollWidth} > ${audit.clientWidth}`);
  if (audit.theme !== shot[3]) throw new Error(`${name}: rendered theme ${audit.theme}, expected ${shot[3]}`);
  if (audit.profile !== shot[5]) throw new Error(`${name}: rendered profile ${audit.profile}, expected ${shot[5]}`);
  if (!audit.heading) throw new Error(`${name}: no page heading rendered`);

  const file = path.join(outDir, `${name}.png`);
  let buf;
  if (verifyOnly) {
    buf = readFileSync(file);
  } else {
    await page.screenshot({path: file});
    buf = readFileSync(file);
  }
  const [w, h] = pngSize(buf);
  if (w !== width || h !== height) throw new Error(`${name}: PNG ${w}x${h}, expected ${width}x${height}`);
  const sha = createHash('sha256').update(buf).digest('hex');
  if (seen.has(sha)) throw new Error(`${name}: duplicate of ${seen.get(sha)}`);
  seen.set(sha, name);
  entries.push({file: `${name}.png`, width, height, theme: shot[3], lang: shot[4], profile: shot[5], scale: shot[6], route: shot[7], sha256: sha});
  console.log(`captured ${name}.png ${w}x${h} <- ${url(shot)}`);
}

await browser.close();
if (verifyOnly && entries.length !== SHOTS.length) throw new Error('missing screenshots');

// The matrix must be exactly the declared set; stale extras abort the run.
if (!verifyOnly) {
  const manifest = {
    captured_at: new Date().toISOString(),
    capture_url: BASE,
    chrome: CHROME,
    screenshots: entries,
  };
  writeFileSync(path.join(outDir, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  console.log(`manifest written: ${entries.length} files, ${seen.size} unique hashes`);
}
if (consoleErrors.length) {
  throw new Error(`console/HTTP errors during capture:\n${[...new Set(consoleErrors)].join('\n')}`);
}
for (const e of entries) {
  if (statSync(path.join(outDir, e.file)).size < 2000) throw new Error(`${e.file}: suspiciously small PNG`);
}
console.log(`OK: ${entries.length} screenshots, ${seen.size} unique, no overflow, no console errors`);
