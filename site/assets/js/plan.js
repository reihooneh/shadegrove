import { PlanError, SCENES, SCENE_IDS, loadScene, plan } from './engine.js';
import { createStreetView } from './street3d.js';
import { SPECIES_COLOURS, clock, compass, download, el, pct, speciesName } from './ui.js';

const $ = (id) => document.getElementById(id);
const view = createStreetView($('plan-view'), { label: 'A 3D view of the street, the planned trees and the footpath coloured by hot sun' });
const SPECIES = [
  ['brush-box', 'Brush Box'], ['plane', 'London Plane'], ['chinese-elm', 'Chinese Elm'],
  ['jacaranda', 'Jacaranda'], ['water-gum', 'Water Gum'], ['crepe-myrtle', 'Crepe Myrtle'],
];

// Each form field: its id, the scene key, and its allowed range (the engine checks again).
const FIELDS = [
  { id: 'temp', key: 'maxTemp', min: 20, max: 55, label: 'Top temperature' },
  { id: 'from', key: 'fromHour', min: 4, max: 22, label: 'From' },
  { id: 'to', key: 'toHour', min: 5, max: 23, label: 'To' },
  { id: 'budget', key: 'budget', min: 1, max: 80, label: 'Trees to plant', whole: true },
  { id: 'year', key: 'targetYear', min: 1, max: 40, label: 'Age', whole: true },
  { id: 'spacing', key: 'minSpacing', min: 2, max: 30, label: 'Space between trees' },
  { id: 'share', key: 'maxShare', min: 10, max: 100, label: 'Most from one species', scale: 100 },
];

let base = null; // the scene as loaded, before the form's changes
let result = null;
let mode = 'after';

for (const s of SCENES) $('scene').append(el('option', { value: s.id, text: s.name }));
$('scene').append(el('option', { value: 'own', text: 'My own scene file', disabled: true }));
for (const [id, name] of SPECIES) {
  const box = el('input', { type: 'checkbox', value: id, id: `sp-${id}`, checked: true });
  $('species').append(el('label', { for: `sp-${id}` }, box, el('span', { class: 'swatch', 'data-species': id, 'aria-hidden': 'true' }), name));
}
// Colour swatches without inline styles: set them from script (allowed by the CSP).
for (const sw of document.querySelectorAll('.swatch[data-species]')) sw.style.background = SPECIES_COLOURS[sw.dataset.species];

/** Reads the URL fragment. Only known keys and in-range numbers are used; anything else is ignored. */
function readHash() {
  const out = {};
  const params = new URLSearchParams(location.hash.slice(1));
  const sc = params.get('scene');
  if (sc && SCENE_IDS.includes(sc)) out.scene = sc;
  for (const f of FIELDS) {
    const raw = params.get(f.key);
    if (raw === null || !/^\d{1,3}(\.\d{1,2})?$/.test(raw)) continue;
    const v = Number(raw);
    if (v >= f.min / (f.scale ?? 1) && v <= f.max / (f.scale ?? 1)) out[f.key] = v;
  }
  return out;
}

function fillForm(scene) {
  $('date').value = scene.date;
  for (const f of FIELDS) {
    const v = scene[f.key];
    const defaults = { maxTemp: 38, fromHour: 9, toHour: 18, budget: 10, targetYear: 10, minSpacing: 7, maxShare: 0.5 };
    $(f.id).value = String(Math.round((v || defaults[f.key]) * (f.scale ?? 1) * 100) / 100);
  }
  const allowed = scene.species?.length ? scene.species : SPECIES.map(([id]) => id);
  for (const [id] of SPECIES) $(`sp-${id}`).checked = allowed.includes(id);
}

/** Builds the scene to plan from the loaded street and the form. Returns null and marks fields if invalid. */
function sceneFromForm() {
  const problems = [];
  const scene = structuredClone(base);
  for (const f of FIELDS) {
    const input = $(f.id);
    const v = Number(input.value);
    const ok = input.value.trim() !== '' && Number.isFinite(v) && v >= f.min && v <= f.max && (!f.whole || Number.isInteger(v));
    input.setAttribute('aria-invalid', ok ? 'false' : 'true');
    if (!ok) problems.push(`${f.label} must be ${f.whole ? 'a whole number ' : ''}between ${f.min} and ${f.max}.`);
    else scene[f.key] = v / (f.scale ?? 1);
  }
  if (scene.toHour < scene.fromHour + 1) {
    $('to').setAttribute('aria-invalid', 'true');
    problems.push('The hours must be at least one hour apart.');
  }
  const date = $('date').value;
  const okDate = /^\d{4}-\d{2}-\d{2}$/.test(date);
  $('date').setAttribute('aria-invalid', okDate ? 'false' : 'true');
  if (!okDate) problems.push('Choose a date.');
  else scene.date = date;
  const species = SPECIES.map(([id]) => id).filter((id) => $(`sp-${id}`).checked);
  if (!species.length) problems.push('Choose at least one species.');
  scene.species = species;
  $('form-error').textContent = problems.join(' ');
  return problems.length ? null : scene;
}

function writeHash(scene, id) {
  const p = new URLSearchParams();
  if (id) p.set('scene', id);
  for (const f of FIELDS) p.set(f.key, String(scene[f.key]));
  history.replaceState(null, '', `#${p}`);
  $('to-grow').href = `grow.html#${p}`;
}

function showTime(i) {
  const m = result.moments[i];
  if (!m) return;
  $('readout').replaceChildren(
    el('span', {}, el('b', { text: clock(m.hour) })),
    el('span', { text: `${m.temp.toFixed(0)} °C` }),
    el('span', { text: `sun ${m.elevation.toFixed(0)}° up in the ${compass(m.azimuth)}` }),
  );
  view.setTime(i);
}

function setMode(m) {
  mode = m;
  $('show-before').setAttribute('aria-pressed', String(m === 'before'));
  $('show-after').setAttribute('aria-pressed', String(m === 'after'));
  if (result) {
    view.setMode(m);
    view.setYear(result.scene.targetYear);
  }
}

function show(r) {
  result = r;
  const p = r.plan;
  let start = r.moments.findIndex((m) => m.hour >= 15);
  if (start < 0) start = Math.max(0, r.moments.length - 1);
  view.setResult(r, { mode, time: start, showSites: true });
  $('time').max = String(Math.max(0, r.moments.length - 1));
  $('time').value = String(start);
  showTime(start);
  $('sum-cut').textContent = pct(p.reduction);
  $('sum-guarantee').textContent = pct(p.guarantee);
  $('sum-winter').textContent = pct(r.seasons.winterLost);
  $('picks').replaceChildren(...p.picks.map((pk) => {
    const sw = el('span', { class: 'swatch', 'aria-hidden': 'true' });
    sw.style.background = SPECIES_COLOURS[pk.species] ?? '#2e6a4c';
    return el('tr', {},
      el('td', { class: 'num', text: String(pk.order) }),
      el('td', { text: pk.site }),
      el('td', {}, sw, speciesName(r, pk.species)),
      el('td', { class: 'num', text: pct(pk.gainShare, 1) }));
  }));
  $('notes').replaceChildren(...r.notes.map((n) => el('li', { text: n })));
  $('status').textContent = `${r.scene.name}: ${p.picks.length} trees, judged at ${r.scene.targetYear} years old on ${r.scene.date}.`;
}

async function run() {
  const scene = sceneFromForm();
  if (!scene) return;
  $('go').disabled = true;
  $('status').textContent = 'Tracing the sun and placing trees…';
  try {
    show(await plan(scene));
    const id = $('scene').value;
    writeHash(scene, SCENE_IDS.includes(id) ? id : null);
  } catch (err) {
    $('status').textContent = err instanceof PlanError ? `The planner refused this scene: ${err.message}` : 'Something went wrong while planning.';
  } finally {
    $('go').disabled = false;
  }
}

async function useScene(id, overrides = {}) {
  base = await loadScene(id);
  $('scene').value = id;
  fillForm({ ...base, ...overrides });
}

$('form').addEventListener('submit', (e) => { e.preventDefault(); run(); });
$('scene').addEventListener('change', async () => { await useScene($('scene').value); run(); });
$('time').addEventListener('input', () => showTime(Number($('time').value)));
$('show-before').addEventListener('click', () => setMode('before'));
$('show-after').addEventListener('click', () => setMode('after'));
$('file').addEventListener('change', async () => {
  const file = $('file').files[0];
  if (!file) return;
  $('form-error').textContent = '';
  if (file.size > 256 * 1024) {
    $('form-error').textContent = 'That file is larger than 256 KB.';
    return;
  }
  let scene;
  try {
    scene = JSON.parse(await file.text());
    if (!scene || typeof scene !== 'object' || Array.isArray(scene)) throw new Error('not an object');
  } catch {
    $('form-error').textContent = 'That file is not a JSON scene.';
    return;
  }
  base = scene;
  const own = $('scene').querySelector('option[value="own"]');
  own.disabled = false;
  $('scene').value = 'own';
  try {
    fillForm({ date: '2026-01-15', ...scene });
  } catch {
    $('form-error').textContent = 'That file is not a JSON scene.';
    return;
  }
  run();
});
$('dl-csv').addEventListener('click', () => {
  if (!result) return;
  // Every value is a number or comes from the engine's own vocabulary (site ids are validated).
  const rows = [['order', 'site', 'x_m', 'y_m', 'species', 'hot_sun_removed_share']];
  for (const p of result.plan.picks) rows.push([p.order, p.site, p.x, p.y, p.species, p.gainShare.toFixed(4)]);
  download('shadegrove-plan.csv', 'text/csv', `${rows.map((r) => r.join(',')).join('\n')}\n`);
});
$('dl-scene').addEventListener('click', () => {
  if (!result) return;
  download('shadegrove-scene.json', 'application/json', `${JSON.stringify(result.scene, null, 1)}\n`);
});

(async () => {
  const fromHash = readHash();
  try {
    await useScene(fromHash.scene ?? 'terrace-street', fromHash);
    await run();
  } catch {
    $('status').textContent = 'The street could not be loaded.';
  }
  document.body.dataset.ready = '1';
})();
