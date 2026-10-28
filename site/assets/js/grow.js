import { SCENE_IDS, loadScene, plan } from './engine.js';
import { createStreetView } from './street3d.js';
import { el, pct, svg } from './ui.js';

const $ = (id) => document.getElementById(id);
const view = createStreetView($('grow-view'), { label: 'A 3D view of the street with the planned trees at the chosen age' });

const STRATEGIES = [
  { id: 'mixed', name: 'Planner’s choice', colour: '#123238', species: [] },
  { id: 'big', name: 'Big shade trees', colour: '#2e6a4c', species: ['plane', 'brush-box'] },
  { id: 'fast', name: 'Fast growers', colour: '#c0577f', species: ['crepe-myrtle', 'jacaranda', 'chinese-elm'] },
  { id: 'evergreen', name: 'Evergreens only', colour: '#7a6a2e', species: ['brush-box', 'water-gum'] },
];

/** The fragment may choose the street and a few numbers; everything else is ignored. */
function readHash() {
  const params = new URLSearchParams(location.hash.slice(1));
  const scene = SCENE_IDS.includes(params.get('scene')) ? params.get('scene') : 'terrace-street';
  const num = (key, lo, hi) => {
    const raw = params.get(key);
    if (raw === null || !/^\d{1,3}(\.\d{1,2})?$/.test(raw)) return undefined;
    const v = Number(raw);
    return v >= lo && v <= hi ? v : undefined;
  };
  return {
    scene,
    overrides: Object.fromEntries(Object.entries({
      budget: num('budget', 1, 80), targetYear: num('targetYear', 1, 40), maxTemp: num('maxTemp', 20, 55),
      minSpacing: num('minSpacing', 2, 30),
    }).filter(([k, v]) => v !== undefined && (!['budget', 'targetYear'].includes(k) || Number.isInteger(v)))),
  };
}

function drawChart(results) {
  const W = 520, H = 300, L = 46, R = 14, T = 14, B = 40;
  const years = results[0].growth.map((g) => g.year);
  const maxYear = Math.max(...years);
  const top = Math.max(0.1, ...results.flatMap((r) => r.growth.map((g) => g.reduction)));
  const yMax = Math.ceil(top * 10) / 10;
  const X = (y) => L + (y / maxYear) * (W - L - R);
  const Y = (v) => T + (1 - v / yMax) * (H - T - B);
  const s = svg('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Hot sun removed against years after planting, for four species choices' });
  for (let v = 0; v <= yMax + 1e-9; v += 0.1) {
    s.append(svg('line', { x1: L, x2: W - R, y1: Y(v), y2: Y(v), class: 'axis' }));
    s.append(svg('text', { x: L - 8, y: Y(v) + 4, 'text-anchor': 'end', text: pct(v) }));
  }
  for (const y of [0, 5, 10, 15, 20, 30].filter((y) => y <= maxYear)) {
    s.append(svg('text', { x: X(y), y: H - B + 18, 'text-anchor': 'middle', text: String(y) }));
  }
  s.append(svg('text', { x: (L + W - R) / 2, y: H - 6, 'text-anchor': 'middle', text: 'years after planting' }));
  results.forEach((r, i) => {
    const pts = [[0, 0], ...r.growth.map((g) => [g.year, g.reduction])];
    s.append(svg('polyline', {
      points: pts.map(([x, v]) => `${X(x).toFixed(1)},${Y(v).toFixed(1)}`).join(' '),
      fill: 'none', stroke: STRATEGIES[i].colour, 'stroke-width': i === 0 ? 3.5 : 2.5, 'stroke-linejoin': 'round',
    }));
  });
  $('chart').replaceChildren(s);
  $('key').replaceChildren(...STRATEGIES.map((st) => {
    const swatch = el('i', { 'aria-hidden': 'true' });
    swatch.style.background = st.colour;
    return el('li', {}, swatch, st.name);
  }));
}

function drawBars(results) {
  $('bars').replaceChildren(...results.map((r, i) => {
    const row = (cls, v, label) => {
      const fill = el('div', { class: `bar-fill ${cls}` });
      fill.style.width = `${Math.min(100, v * 100).toFixed(1)}%`;
      return el('div', { class: 'bar-row' }, el('span', { text: label }), el('div', { class: 'bar-track', 'aria-hidden': 'true' }, fill), el('span', { text: pct(v) }));
    };
    return el('div', { class: 'bar-group' },
      el('h4', { text: STRATEGIES[i].name }),
      row('summer', r.seasons.summerRemoved, 'Summer shade'),
      row('winter', r.seasons.winterLost, 'Winter sun lost'));
  }));
}

(async () => {
  try {
    const { scene: id, overrides } = readHash();
    const base = { ...(await loadScene(id)), ...overrides };
    const results = [];
    for (const st of STRATEGIES) results.push(await plan({ ...base, species: st.species }));
    const mixed = results[0];
    const years = mixed.years;
    const age = $('age');
    age.max = String(years.length - 1);
    let start = years.indexOf(mixed.scene.targetYear);
    if (start < 0) start = years.length - 1;
    age.value = String(start);
    let noon = mixed.moments.findIndex((m) => m.hour >= 14);
    if (noon < 0) noon = 0;
    view.setResult(mixed, { mode: 'after', year: years[start], time: noon });
    const showAge = () => {
      const y = years[Number(age.value)];
      const g = mixed.growth.find((x) => x.year === y);
      view.setYear(y);
      $('age-readout').replaceChildren(
        el('span', {}, el('b', { text: y === 0 ? 'Planting day' : `${y} ${y === 1 ? 'year' : 'years'}` })),
        el('span', { text: y === 0 ? 'young trees, 2.5 m tall' : g ? `${pct(g.reduction)} less hot sun on the footpaths` : '' }),
      );
    };
    age.addEventListener('input', showAge);
    showAge();
    drawChart(results);
    drawBars(results);
    $('status').textContent = `${mixed.scene.name}, ${mixed.scene.budget} trees. The 3D view shows the planner’s choice.`;
  } catch (err) {
    $('status').textContent = `The planner could not run here: ${err.message}`;
  }
  document.body.dataset.ready = '1';
})();
