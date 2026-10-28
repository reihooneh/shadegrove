import { SCENES, loadScene, plan } from './engine.js';
import { createStreetView } from './street3d.js';
import { clock, compass, el, pct, reducedMotion } from './ui.js';

const view = createStreetView(document.getElementById('hero-view'), { label: 'A 3D view of a terrace street with the planned trees and the footpath coloured by hot sun' });
const time = document.getElementById('hero-time');
const readout = document.getElementById('hero-readout');
const status = document.getElementById('home-status');

for (const s of SCENES) {
  document.getElementById('street-links').append(
    el('a', { class: 'street', href: `plan.html#scene=${s.id}` }, el('h3', { text: s.name }), el('p', { text: s.blurb })),
  );
}

function showTime(result, i) {
  const m = result.moments[i];
  readout.replaceChildren(
    el('span', {}, el('b', { text: clock(m.hour) })),
    el('span', { text: `${m.temp.toFixed(0)} °C` }),
    el('span', { text: `sun ${m.elevation.toFixed(0)}° up in the ${compass(m.azimuth)}` }),
  );
  view.setTime(i);
}

async function start() {
  try {
    const result = await plan(await loadScene('terrace-street'));
    const p = result.plan;
    // Start in the mid-afternoon heat.
    let start = result.moments.findIndex((m) => m.hour >= 15);
    if (start < 0) start = 0;
    view.setResult(result, { mode: 'after', time: start });
    time.max = String(result.moments.length - 1);
    time.value = String(start);
    showTime(result, start);
    time.addEventListener('input', () => showTime(result, Number(time.value)));
    document.getElementById('fact-cut').textContent = pct(p.reduction);
    document.getElementById('fact-guarantee').textContent = pct(p.guarantee);
    document.getElementById('fact-winter').textContent = pct(result.seasons.winterLost);
    status.textContent = `Planned in your browser: ${p.picks.length} trees chosen from ${p.candidates} site and species options.`;
    if (!reducedMotion()) {
      // One slow sweep of the afternoon, then it stops and the slider is yours.
      let i = start;
      const timer = setInterval(() => {
        i += 1;
        if (i >= result.moments.length || document.activeElement === time) return clearInterval(timer);
        time.value = String(i);
        showTime(result, i);
      }, 450);
      time.addEventListener('pointerdown', () => clearInterval(timer), { once: true });
    }
  } catch (err) {
    status.textContent = `The planner could not run here: ${err.message}`;
    status.classList.add('error');
  }
  document.body.dataset.ready = '1';
}
start();
