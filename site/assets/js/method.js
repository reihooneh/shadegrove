import { loadScene, plan } from './engine.js';
import { el, pct } from './ui.js';

const $ = (id) => document.getElementById(id);

// A tiny open scene: just enough for the engine to report the sun's height through the day.
const probe = (date, utcOffset) => ({
  name: 'Sun check', latitude: -33.87, longitude: 151.21, utcOffset, date, fromHour: 6, toHour: 20, stepMinutes: 10,
  walks: [{ x0: 0, y0: 0, x1: 2, y1: 2 }], sites: [{ id: 'A', x: 1, y: 1 }], budget: 1,
});

const highest = (r) => Math.max(...r.moments.map((m) => m.elevation));

(async () => {
  try {
    const summer = await plan(probe('2026-12-21', 11));
    const winter = await plan(probe('2026-06-21', 10));
    // Noon height by hand: 90° − (latitude difference from the sun's declination of ±23.44°).
    const rows = [
      ['Highest sun at Sydney, 21 December', 90 - (33.87 - 23.44), highest(summer)],
      ['Highest sun at Sydney, 21 June', 90 - (33.87 + 23.44), highest(winter)],
    ];
    $('check').replaceChildren(...rows.map(([what, hand, got]) => el('tr', {},
      el('td', { text: what }), el('td', { class: 'num', text: `${hand.toFixed(1)}°` }), el('td', { class: 'num', text: `${got.toFixed(1)}°` }))));

    const street = await plan(await loadScene('terrace-street'));
    const p = street.plan;
    $('bound').replaceChildren(
      el('tr', {}, el('td', { text: 'Hot sun the plan removes' }), el('td', { class: 'num', text: pct(p.reduction, 1) })),
      el('tr', {}, el('td', { text: 'Most any plan of the same size could remove' }), el('td', { class: 'num', text: pct(p.upperBound / p.before, 1) })),
      el('tr', {}, el('td', { text: 'So the plan is at least this share of the best' }), el('td', { class: 'num', text: pct(p.guarantee, 1) })),
    );
  } catch (err) {
    $('check').replaceChildren(el('tr', {}, el('td', { text: `The planner could not run here: ${err.message}` })));
  }
  document.body.dataset.ready = '1';
})();
