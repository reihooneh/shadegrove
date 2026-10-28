// Browser tests for every page at phone, tablet and desktop widths: no errors, no sideways
// scrolling, no text on top of other text (HTML or SVG), and the main things work.
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { chromium } from 'playwright';
import { serve } from '../../scripts/serve.mjs';

const PAGES = ['index.html', 'plan.html', 'grow.html', 'method.html'];
const SIZES = [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'tablet', width: 820, height: 1180 },
  { name: 'phone', width: 390, height: 844 },
];
let server, browser, base;

before(async () => {
  server = await serve();
  base = `http://127.0.0.1:${server.address().port}/`;
  browser = await chromium.launch({ args: ['--use-gl=swiftshader', '--enable-unsafe-swiftshader'] });
});
after(async () => {
  await browser?.close();
  server?.close();
});

async function open(path, size = SIZES[0], opts = {}) {
  const context = await browser.newContext({ viewport: { width: size.width, height: size.height }, reducedMotion: 'reduce', acceptDownloads: true, ...opts });
  const page = await context.newPage();
  const errors = [];
  page.on('console', (m) => {
    if (m.type() === 'error' && !/GL Driver Message|WebGL/.test(m.text())) errors.push(m.text());
  });
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(base + path);
  await page.waitForFunction(() => document.body.dataset.ready, null, { timeout: 60000 });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
  return { page, context, errors };
}

// Every piece of visible text, HTML or SVG: no two different pieces overlap.
async function textOverlaps(page) {
  return page.evaluate(() => {
    const boxes = [];
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const n = walker.currentNode;
      if (!n.textContent.trim()) continue;
      const elx = n.parentElement;
      if (elx.closest('.visually-hidden, [hidden], .skip, option, title, select')) continue;
      const cs = getComputedStyle(elx);
      if (cs.visibility === 'hidden' || cs.display === 'none' || Number(cs.opacity) === 0) continue;
      if (elx instanceof SVGElement) {
        const r = elx.getBoundingClientRect();
        if (r.width >= 1 && r.height >= 1) boxes.push({ r, text: n.textContent.trim().slice(0, 40), node: n });
        continue;
      }
      const range = document.createRange();
      range.selectNodeContents(n);
      for (const r of range.getClientRects()) if (r.width >= 1 && r.height >= 1) boxes.push({ r, text: n.textContent.trim().slice(0, 40), node: n });
    }
    const out = [];
    for (let i = 0; i < boxes.length; i++) {
      for (let j = i + 1; j < boxes.length; j++) {
        const a = boxes[i], b = boxes[j];
        if (a.node === b.node) continue;
        const w = Math.min(a.r.right, b.r.right) - Math.max(a.r.left, b.r.left);
        const h = Math.min(a.r.bottom, b.r.bottom) - Math.max(a.r.top, b.r.top);
        if (w > 2 && h > Math.min(a.r.height, b.r.height) * 0.35) out.push(`"${a.text}" overlaps "${b.text}"`);
      }
    }
    return out;
  });
}

// SVG text must stay inside its drawing, and shapes inside a chart must stay inside it too.
async function svgOutside(page) {
  return page.evaluate(() => {
    const out = [];
    for (const s of document.querySelectorAll('.figure svg, .chart svg')) {
      const f = s.getBoundingClientRect();
      for (const t of s.querySelectorAll('text, line, rect, polyline, ellipse, circle')) {
        const r = t.getBoundingClientRect();
        if (r.width === 0 && r.height === 0) continue;
        if (r.left < f.left - 1 || r.right > f.right + 1 || r.top < f.top - 1 || r.bottom > f.bottom + 1) out.push(t.textContent || t.tagName);
      }
    }
    return out;
  });
}

// Controls must not sit on top of each other (buttons, inputs, the 3D view, tables).
async function boxOverlaps(page) {
  return page.evaluate(() => {
    const els = [...document.querySelectorAll('main .button, main input, main select, main .view, main table, main .chart')]
      .filter((e) => e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden');
    const out = [];
    for (let i = 0; i < els.length; i++) {
      for (let j = i + 1; j < els.length; j++) {
        if (els[i].contains(els[j]) || els[j].contains(els[i])) continue;
        const a = els[i].getBoundingClientRect(), b = els[j].getBoundingClientRect();
        const w = Math.min(a.right, b.right) - Math.max(a.left, b.left);
        const h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (w > 1 && h > 1) out.push(`${els[i].id || els[i].className} overlaps ${els[j].id || els[j].className}`);
      }
    }
    return out;
  });
}

for (const path of PAGES) {
  for (const size of SIZES) {
    test(`${path} at ${size.name}: no errors, no sideways scroll, nothing overlapping`, async () => {
      const { page, context, errors } = await open(path, size);
      const scroll = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      assert.ok(scroll <= 0, `page scrolls sideways by ${scroll}px`);
      assert.deepEqual(await textOverlaps(page), []);
      assert.deepEqual(await svgOutside(page), []);
      assert.deepEqual(await boxOverlaps(page), []);
      assert.deepEqual(errors, []);
      await context.close();
    });
  }
}

test('built pages: strict CSP, no inline code, nothing loaded from other sites', () => {
  const dir = new URL('../../site/', import.meta.url);
  for (const f of readdirSync(dir).filter((x) => x.endsWith('.html'))) {
    const html = readFileSync(new URL(f, dir), 'utf8');
    assert.match(html, /Content-Security-Policy" content="default-src 'none'; script-src 'self' 'wasm-unsafe-eval';/, f);
    assert.doesNotMatch(html, /'unsafe-inline'|'unsafe-eval'/, f);
    assert.doesNotMatch(html, /<script(?![^>]*\bsrc=)[^>]*>/, `${f} has an inline script`);
    assert.doesNotMatch(html, /\sstyle=|<style|\son[a-z]+=/i, `${f} has inline styles or handlers`);
    const external = [...html.matchAll(/(?:src|href)="(https?:[^"]+)"/g)].map((m) => m[1]);
    for (const url of external) assert.ok(/^https:\/\/github\.com\/reihooneh\//.test(url), `${f} links ${url}`);
  }
  for (const f of readdirSync(new URL('assets/js/', dir))) {
    const js = readFileSync(new URL(`assets/js/${f}`, dir), 'utf8');
    assert.doesNotMatch(js, /innerHTML|outerHTML|insertAdjacentHTML|document\.write|\beval\(|new Function/, `${f} builds HTML from strings`);
  }
});

test('home: the street, the plan numbers and the time of day', async () => {
  const { page, context, errors } = await open('index.html');
  assert.ok(['webgl', 'flat'].includes(await page.$eval('#hero-view', (n) => n.dataset.mode)));
  assert.match(await page.textContent('#fact-cut'), /^\d+%$/);
  assert.match(await page.textContent('#home-status'), /10 trees chosen/);
  await page.$eval('#hero-time', (n) => { n.value = '0'; n.dispatchEvent(new Event('input')); });
  assert.match(await page.textContent('#hero-readout'), /9:0\d am/);
  assert.equal(await page.locator('.street').count(), 3);
  assert.deepEqual(errors, []);
  await context.close();
});

test('home: works without WebGL (flat map)', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, reducedMotion: 'reduce' });
  await context.addInitScript(() => {
    const orig = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (type, ...rest) {
      return /webgl/.test(type) ? null : orig.call(this, type, ...rest);
    };
  });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(`${base}index.html`);
  await page.waitForFunction(() => document.body.dataset.ready, null, { timeout: 60000 });
  assert.equal(await page.$eval('#hero-view', (n) => n.dataset.mode), 'flat');
  assert.deepEqual(errors, []);
  await context.close();
});

test('planner: change the street and the budget, toggle the view, download the plan', async () => {
  const { page, context, errors } = await open('plan.html');
  assert.equal(await page.locator('#picks tr').count(), 10);
  await page.selectOption('#scene', 'city-avenue');
  await page.waitForFunction(() => /City avenue/.test(document.getElementById('status').textContent), null, { timeout: 60000 });
  await page.fill('#budget', '4');
  await page.click('#go');
  await page.waitForFunction(() => document.querySelectorAll('#picks tr').length === 4, null, { timeout: 60000 });
  assert.match(page.url(), /#scene=city-avenue&.*budget=4/);
  const shares = await page.$$eval('#picks tr td:last-child', (t) => t.map((x) => parseFloat(x.textContent)));
  for (let i = 1; i < shares.length; i++) assert.ok(shares[i] <= shares[i - 1] + 1e-9, 'gains shrink as the plan grows');
  await page.click('#show-before');
  assert.equal(await page.getAttribute('#show-before', 'aria-pressed'), 'true');
  assert.deepEqual(await textOverlaps(page), []);
  const [csv] = await Promise.all([page.waitForEvent('download'), page.click('#dl-csv')]);
  const lines = readFileSync(await csv.path(), 'utf8').trim().split('\n');
  assert.equal(lines[0], 'order,site,x_m,y_m,species,hot_sun_removed_share');
  assert.equal(lines.length, 5);
  assert.match(await page.getAttribute('#to-grow', 'href'), /^grow\.html#scene=city-avenue/);
  assert.deepEqual(errors, []);
  await context.close();
});

test('planner: species limits and the mix rule are respected', async () => {
  const { page, context, errors } = await open('plan.html#scene=town-square&budget=6');
  for (const id of ['plane', 'brush-box', 'chinese-elm', 'jacaranda']) await page.uncheck(`#sp-${id}`);
  await page.fill('#share', '50');
  await page.click('#go');
  await page.waitForFunction(() => /Town square/.test(document.getElementById('status').textContent) && document.querySelectorAll('#picks tr').length > 0, null, { timeout: 60000 });
  const species = await page.$$eval('#picks tr td:nth-child(3)', (t) => t.map((x) => x.textContent));
  assert.ok(species.every((s) => /Water Gum|Crepe Myrtle/.test(s)), species.join());
  for (const s of ['Water Gum', 'Crepe Myrtle']) assert.ok(species.filter((x) => x === s).length <= 3, `${s} over half`);
  assert.deepEqual(errors, []);
  await context.close();
});

test('planner: bad numbers are explained; a crafted link and a hostile file are refused', async () => {
  const { page, context, errors } = await open('plan.html#scene=%3Cimg%20src=x%3E&budget=99999&maxTemp=__proto__');
  assert.equal(await page.inputValue('#scene'), 'terrace-street');
  assert.equal(await page.inputValue('#budget'), '10');
  await page.fill('#temp', '80');
  await page.click('#go');
  assert.match(await page.textContent('#form-error'), /Top temperature must be between 20 and 55/);
  assert.equal(await page.getAttribute('#temp', 'aria-invalid'), 'true');
  await page.fill('#temp', '38');
  for (const id of ['brush-box', 'plane', 'chinese-elm', 'jacaranda', 'water-gum', 'crepe-myrtle']) await page.uncheck(`#sp-${id}`);
  await page.click('#go');
  assert.match(await page.textContent('#form-error'), /at least one species/);
  await page.check('#sp-plane');

  const dir = mkdtempSync(join(tmpdir(), 'shadegrove-'));
  try {
    const junk = join(dir, 'scene.json');
    writeFileSync(junk, 'not json <script>alert(1)</script>');
    await page.setInputFiles('#file', junk);
    assert.match(await page.textContent('#form-error'), /not a JSON scene/);
    const hostile = join(dir, 'hostile.json');
    writeFileSync(hostile, JSON.stringify({ name: '<img src=x onerror=alert(1)>', date: '2026-01-15', walks: [{ x0: 0, y0: 0, x1: 4, y1: 4 }], sites: [{ id: 'A', x: 1, y: 1 }] }));
    await page.setInputFiles('#file', hostile);
    await page.waitForFunction(() => /refused/.test(document.getElementById('status').textContent), null, { timeout: 60000 });
    assert.match(await page.textContent('#status'), /The planner refused this scene: the name uses letters/);
    const own = join(dir, 'own.json');
    writeFileSync(own, JSON.stringify({ name: 'My street', latitude: -27.47, longitude: 153.02, utcOffset: 10, date: '2026-01-20',
      walks: [{ x0: -20, y0: 0, x1: 20, y1: 3 }], sites: [{ id: 'A', x: -10, y: -1 }, { id: 'B', x: 10, y: -1 }], budget: 2 }));
    await page.setInputFiles('#file', own);
    await page.waitForFunction(() => /My street/.test(document.getElementById('status').textContent), null, { timeout: 60000 });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
  assert.equal(await page.$('img[src="x"]'), null);
  assert.deepEqual(errors, []);
  await context.close();
});

test('grow: four strategies, the years slider and the seasons', async () => {
  const { page, context, errors } = await open('grow.html#scene=terrace-street');
  assert.equal(await page.locator('#chart polyline').count(), 4);
  assert.equal(await page.locator('.bar-group').count(), 4);
  await page.$eval('#age', (n) => { n.value = '0'; n.dispatchEvent(new Event('input')); });
  assert.match(await page.textContent('#age-readout'), /Planting day/);
  await page.$eval('#age', (n) => { n.value = n.max; n.dispatchEvent(new Event('input')); });
  assert.match(await page.textContent('#age-readout'), /30 years/);
  assert.deepEqual(errors, []);
  await context.close();
});

test('method: the live sun checks agree with geometry by hand', async () => {
  const { page, context, errors } = await open('method.html');
  const rows = await page.$$eval('#check tr', (r) => r.map((x) => [...x.querySelectorAll('td')].map((td) => parseFloat(td.textContent))));
  assert.equal(rows.length, 2);
  for (const [, hand, got] of rows) assert.ok(Math.abs(hand - got) < 0.4, `${hand} vs ${got}`);
  assert.equal(await page.locator('#bound tr').count(), 3);
  assert.deepEqual(errors, []);
  await context.close();
});
