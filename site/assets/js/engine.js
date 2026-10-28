// Talks to the planner (Go compiled to WebAssembly) running in a Web Worker.

export const SCENES = [
  { id: 'terrace-street', name: 'Terrace main street', blurb: 'An east–west shopping street of two-storey terraces, with power lines over the south footpath.' },
  { id: 'city-avenue', name: 'City avenue', blurb: 'A north–south avenue between taller buildings, busiest at the station end.' },
  { id: 'town-square', name: 'Town square', blurb: 'An open square with a playground in its sunniest corner and one old plane tree.' },
];
export const SCENE_IDS = SCENES.map((s) => s.id);

let worker = null;
let seq = 0;
const waiting = new Map();
const cache = new Map();

function getWorker() {
  if (!worker) {
    worker = new Worker(new URL('./worker.js', import.meta.url));
    worker.onmessage = (event) => {
      const { id, result } = event.data || {};
      const done = waiting.get(id);
      if (!done) return;
      waiting.delete(id);
      done(result);
    };
  }
  return worker;
}

export class PlanError extends Error {}

/** Runs the planner on a scene object. Resolves with the result, or rejects with a PlanError. */
export async function plan(scene) {
  const text = JSON.stringify(scene);
  if (cache.has(text)) return cache.get(text);
  const id = ++seq;
  const raw = await new Promise((resolve) => {
    waiting.set(id, resolve);
    getWorker().postMessage({ id, scene: text });
  });
  let result;
  try {
    result = JSON.parse(raw);
  } catch {
    throw new PlanError('The planner sent back something unreadable.');
  }
  if (result.error) throw new PlanError(result.error);
  cache.set(text, result);
  return result;
}

/** Loads one of the bundled scenes by id (only ids from the list above). */
export async function loadScene(id) {
  if (!SCENE_IDS.includes(id)) throw new PlanError('Unknown street.');
  const res = await fetch(new URL(`../scenes/${id}.json`, import.meta.url));
  if (!res.ok) throw new PlanError('The street could not be loaded.');
  return res.json();
}
