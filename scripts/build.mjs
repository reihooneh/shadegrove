// Builds the site from its sources:
//   src/pages/*.html + the shared layout  ->  site/*.html
//   scenes/*.json                         ->  site/assets/scenes/
//   three.js and the fonts (pinned npm)   ->  site/assets/vendor, site/assets/fonts
//   Go's wasm_exec.js (from the Go used)  ->  site/assets/vendor/wasm_exec.js
// The WebAssembly engine itself is built by `make wasm`. CI checks the committed site matches.
import { execFileSync } from 'node:child_process';
import { copyFileSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const goroot = execFileSync('go', ['env', 'GOROOT'], { encoding: 'utf8' }).trim();

// ---------- Vendored and copied files ----------
const copies = [
  ['node_modules/three/build/three.module.min.js', 'site/assets/vendor/three.module.min.js'],
  ['node_modules/three/LICENSE', 'site/assets/vendor/three.LICENSE.txt'],
  [join(goroot, 'lib/wasm/wasm_exec.js'), 'site/assets/vendor/wasm_exec.js'],
  [join(goroot, 'LICENSE'), 'site/assets/vendor/go.LICENSE.txt'],
  ['node_modules/@fontsource/barlow-condensed/files/barlow-condensed-latin-600-normal.woff2', 'site/assets/fonts/barlow-condensed-600.woff2'],
  ['node_modules/@fontsource/barlow-condensed/files/barlow-condensed-latin-700-normal.woff2', 'site/assets/fonts/barlow-condensed-700.woff2'],
  ['node_modules/@fontsource/barlow-condensed/LICENSE', 'site/assets/fonts/barlow-condensed.LICENSE.txt'],
  ['node_modules/@fontsource-variable/figtree/files/figtree-latin-wght-normal.woff2', 'site/assets/fonts/figtree.woff2'],
  ['node_modules/@fontsource-variable/figtree/files/figtree-latin-wght-italic.woff2', 'site/assets/fonts/figtree-italic.woff2'],
  ['node_modules/@fontsource-variable/figtree/LICENSE', 'site/assets/fonts/figtree.LICENSE.txt'],
];
for (const f of readdirSync(join(root, 'scenes')).filter((x) => x.endsWith('.json'))) {
  copies.push([`scenes/${f}`, `site/assets/scenes/${f}`]);
}
for (const [from, to] of copies) {
  const src = from.startsWith('/') ? from : join(root, from);
  mkdirSync(dirname(join(root, to)), { recursive: true });
  copyFileSync(src, join(root, to));
}

// ---------- Pages ----------
const CSP = [
  "default-src 'none'",
  "script-src 'self' 'wasm-unsafe-eval'",
  "worker-src 'self'",
  "style-src 'self'",
  "img-src 'self' data: blob:",
  "font-src 'self'",
  "connect-src 'self'",
  "base-uri 'none'",
  "form-action 'none'",
].join('; ');
const NAV = [
  ['index', 'index.html', 'Home'],
  ['plan', 'plan.html', 'Plan a street'],
  ['grow', 'grow.html', 'Watch it grow'],
  ['method', 'method.html', 'How it works'],
];
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const pagesDir = join(root, 'src/pages');
const built = [];
for (const file of readdirSync(pagesDir).filter((f) => f.endsWith('.html')).sort()) {
  const raw = readFileSync(join(pagesDir, file), 'utf8');
  const m = raw.match(/^<!--(\{[\s\S]*?\})-->\n/);
  if (!m) throw new Error(`${file}: missing the JSON header comment`);
  const meta = JSON.parse(m[1]);
  const body = raw.slice(m[0].length);
  const nav = NAV.map(
    ([id, href, label]) => `<a href="${href}"${id === meta.nav ? ' aria-current="page"' : ''}>${label}</a>`,
  ).join('\n        ');
  const html = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="${CSP}">
  <meta name="referrer" content="no-referrer">
  <title>${esc(meta.title)}</title>
  <meta name="description" content="${esc(meta.description)}">
  <link rel="icon" href="assets/icon.svg" type="image/svg+xml">
  <link rel="stylesheet" href="assets/css/site.css">
  <script type="module" src="assets/js/${meta.script}"></script>
</head>
<body class="page-${meta.nav}">
  <a class="skip" href="#main">Skip to the content</a>
  <header class="top">
    <div class="top-inner">
      <a class="mark" href="index.html" aria-label="Shadegrove home">
        <svg class="mark-tree" viewBox="0 0 32 32" aria-hidden="true"><circle cx="16" cy="12" r="10"/><rect x="14.5" y="18" width="3" height="11" rx="1"/></svg>
        <span>Shadegrove</span>
      </a>
      <nav class="nav" aria-label="Pages">
        ${nav}
      </nav>
    </div>
  </header>
  <main id="main">
${body.trimEnd()}
  </main>
  <footer class="foot">
    <div class="foot-inner">
      <p>Shadegrove is open source (MIT). It runs entirely in your browser: no sign-in, no tracking, and scenes you load never leave your device.</p>
      <p><a href="https://github.com/reihooneh/shadegrove">Source code and tests on GitHub</a></p>
    </div>
  </footer>
</body>
</html>
`;
  writeFileSync(join(root, 'site', file), html);
  built.push(file);
}
console.log(`Built ${built.length} pages (${built.join(', ')}) and copied ${copies.length} files.`);
