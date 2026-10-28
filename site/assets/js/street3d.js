// The street in 3D: buildings, trees, the footpath coloured by how much hot sun it gets, and the
// sun's light and shadows at the chosen moment. Falls back to a flat map without WebGL.
// Scene coordinates are metres with x east-ish, y north-ish and z up; three.js uses y up, so a
// scene point (x, y, z) is drawn at (x, z, -y).
import * as THREE from '../vendor/three.module.min.js';
import { SPECIES_COLOURS } from './ui.js';

const RAMP = [[0, '#2b6f7a'], [0.35, '#9bc3a6'], [0.7, '#f2c230'], [1, '#e4682b']];
const BUILDING_COLOURS = ['#c8b494', '#b98a6c', '#d6cfbf', '#a8b0a8', '#c49a7c', '#ddd5c2'];

export function rampColour(t) {
  const v = Math.max(0, Math.min(1, t));
  for (let i = 1; i < RAMP.length; i++) {
    if (v <= RAMP[i][0]) {
      const [a, ca] = RAMP[i - 1];
      const [b, cb] = RAMP[i];
      return new THREE.Color(ca).lerp(new THREE.Color(cb), (v - a) / (b - a));
    }
  }
  return new THREE.Color(RAMP[RAMP.length - 1][1]);
}

function webglAvailable() {
  try {
    const c = document.createElement('canvas');
    return Boolean(c.getContext('webgl2') || c.getContext('webgl'));
  } catch {
    return false;
  }
}

/** Crown for a tree at an age: the nearest age the result has. */
export function crownAt(tree, year) {
  let best = tree.crowns[0];
  for (const c of tree.crowns) if (Math.abs(c.year - year) < Math.abs(best.year - year)) best = c;
  return best;
}

function extent(result) {
  let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity;
  const add = (x, y) => { x0 = Math.min(x0, x); x1 = Math.max(x1, x); y0 = Math.min(y0, y); y1 = Math.max(y1, y); };
  for (const w of result.scene.walks) { add(w.x0, w.y0); add(w.x1, w.y1); }
  for (const b of result.scene.buildings) { add(b.x, b.y); add(b.x + b.w, b.y + b.d); }
  return { x0, x1, y0, y1, cx: (x0 + x1) / 2, cy: (y0 + y1) / 2, size: Math.max(x1 - x0, y1 - y0, 20) };
}

export function createStreetView(container, { label = 'A 3D view of the street' } = {}) {
  container.setAttribute('role', 'img');
  container.setAttribute('aria-label', label);
  container.tabIndex = 0;
  const state = { result: null, mode: 'after', year: null, time: 0, showSites: false };
  const view = webglAvailable() ? webgl(container, state) : flat(container, state);
  container.dataset.mode = view.kind;
  return {
    kind: view.kind,
    setResult(result, opts = {}) {
      state.result = result;
      state.year = opts.year ?? result.scene.targetYear;
      state.mode = opts.mode ?? state.mode;
      state.showSites = Boolean(opts.showSites);
      state.time = Math.min(state.time, Math.max(0, result.moments.length - 1));
      if (opts.time !== undefined) state.time = opts.time;
      view.rebuild();
    },
    setMode(mode) { state.mode = mode; view.recolour(); },
    setYear(year) { state.year = year; view.retree(); },
    setTime(i) { state.time = i; view.relight(); },
  };
}

// ---------- WebGL ----------
function webgl(container, state) {
  const renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = THREE.PCFSoftShadowMap;
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  container.append(renderer.domElement);

  const scene = new THREE.Scene();
  scene.background = new THREE.Color('#bfd0cc');
  const camera = new THREE.PerspectiveCamera(38, 1, 0.5, 2000);
  const hemi = new THREE.HemisphereLight('#f4f7f2', '#7d8a80', 1.1);
  scene.add(hemi);
  const sun = new THREE.DirectionalLight('#fff3d6', 2.6);
  sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  sun.shadow.bias = -0.0004;
  sun.shadow.normalBias = 0.02;
  scene.add(sun, sun.target);

  let world = new THREE.Group();
  scene.add(world);
  let cells = null;
  let trees = new THREE.Group();
  let box = null;
  const orbit = { yaw: -0.55, pitch: 0.55, dist: 120 };

  function placeCamera() {
    if (!box) return;
    const t = new THREE.Vector3(box.cx, 0, -box.cy);
    camera.position.set(
      t.x + orbit.dist * Math.cos(orbit.pitch) * Math.sin(orbit.yaw),
      t.y + orbit.dist * Math.sin(orbit.pitch),
      t.z + orbit.dist * Math.cos(orbit.pitch) * Math.cos(orbit.yaw),
    );
    camera.lookAt(t);
  }

  function resize() {
    const w = Math.max(1, container.clientWidth);
    const h = Math.max(1, container.clientHeight);
    renderer.setSize(w, h, false);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
    render();
  }

  let pending = false;
  function render() {
    if (pending) return;
    pending = true;
    requestAnimationFrame(() => {
      pending = false;
      renderer.render(scene, camera);
    });
  }

  const dispose = (obj) => obj.traverse((o) => {
    o.geometry?.dispose();
    if (o.material) (Array.isArray(o.material) ? o.material : [o.material]).forEach((m) => m.dispose());
  });

  function rebuild() {
    const r = state.result;
    scene.remove(world);
    dispose(world);
    world = new THREE.Group();
    scene.add(world);
    box = extent(r);
    orbit.dist = box.size * 0.72;
    // Look along the street, from above one end, so the footpaths show between the buildings.
    const along = box.x1 - box.x0 >= box.y1 - box.y0;
    orbit.yaw = along ? 1.12 : 0.42;
    orbit.pitch = 0.82;
    scene.fog = new THREE.Fog('#bfd0cc', box.size * 1.4, box.size * 3.2);

    const ground = new THREE.Mesh(new THREE.PlaneGeometry(box.size * 10, box.size * 10), new THREE.MeshStandardMaterial({ color: '#6c7774', roughness: 1 }));
    ground.rotation.x = -Math.PI / 2;
    ground.position.set(box.cx, -0.02, -box.cy);
    ground.receiveShadow = true;
    world.add(ground);

    const bmat = BUILDING_COLOURS.map((c) => new THREE.MeshStandardMaterial({ color: c, roughness: 0.9 }));
    r.scene.buildings.forEach((b, i) => {
      const m = new THREE.Mesh(new THREE.BoxGeometry(b.w, b.h, b.d), bmat[i % bmat.length]);
      m.position.set(b.x + b.w / 2, b.h / 2, -(b.y + b.d / 2));
      m.castShadow = true;
      m.receiveShadow = true;
      world.add(m);
    });

    const s = r.cellSize * 0.94;
    const geo = new THREE.BoxGeometry(s, 0.06, s);
    cells = new THREE.InstancedMesh(geo, new THREE.MeshLambertMaterial(), r.cellX.length);
    cells.receiveShadow = true;
    const m4 = new THREE.Matrix4();
    for (let i = 0; i < r.cellX.length; i++) {
      m4.makeTranslation(r.cellX[i], 0.03, -r.cellY[i]);
      cells.setMatrixAt(i, m4);
    }
    world.add(cells);
    recolour(false);

    if (state.showSites) {
      const ring = new THREE.RingGeometry(0.45, 0.75, 20);
      const rmat = new THREE.MeshBasicMaterial({ color: '#123238', side: THREE.DoubleSide });
      for (const site of r.scene.sites) {
        const m = new THREE.Mesh(ring, rmat);
        m.rotation.x = -Math.PI / 2;
        m.position.set(site.x, 0.08, -site.y);
        world.add(m);
      }
    }

    const sb = box.size * 0.9;
    Object.assign(sun.shadow.camera, { left: -sb, right: sb, top: sb, bottom: -sb, near: 1, far: 800 });
    sun.shadow.camera.updateProjectionMatrix();
    retree(false);
    relight(false);
    placeCamera();
    resize();
  }

  function recolour(draw = true) {
    const r = state.result;
    if (!cells) return;
    const values = state.mode === 'before' ? r.cellBefore : r.cellAfter;
    let max = 0;
    for (const v of r.cellBefore) max = Math.max(max, v);
    for (let i = 0; i < values.length; i++) cells.setColorAt(i, rampColour(max > 0 ? values[i] / max : 0));
    cells.instanceColor.needsUpdate = true;
    if (draw) render();
  }

  function retree(draw = true) {
    world.remove(trees);
    dispose(trees);
    trees = new THREE.Group();
    world.add(trees);
    const r = state.result;
    const showPlan = state.mode !== 'before';
    for (const t of r.trees) {
      if (!t.existing && !showPlan) continue;
      const c = crownAt(t, t.existing ? 0 : state.year);
      const colour = new THREE.Color(SPECIES_COLOURS[t.species] ?? '#2e6a4c');
      if (t.existing) colour.multiplyScalar(0.75);
      const trunkH = Math.max(0.5, c.z - c.v * 0.6);
      const trunk = new THREE.Mesh(new THREE.CylinderGeometry(0.12 + c.r * 0.03, 0.18 + c.r * 0.04, trunkH, 8), new THREE.MeshStandardMaterial({ color: '#5a4636' }));
      trunk.position.set(t.x, trunkH / 2, -t.y);
      trunk.castShadow = true;
      const crown = new THREE.Mesh(new THREE.SphereGeometry(1, 20, 14), new THREE.MeshStandardMaterial({ color: colour, roughness: 0.85 }));
      crown.scale.set(c.r, c.v, c.r);
      crown.position.set(t.x, c.z, -t.y);
      crown.castShadow = true;
      crown.receiveShadow = true;
      trees.add(trunk, crown);
    }
    if (draw) render();
  }

  function relight(draw = true) {
    const r = state.result;
    const m = r.moments[state.time];
    if (!m || !box) return;
    const target = new THREE.Vector3(box.cx, 0, -box.cy);
    sun.target.position.copy(target);
    sun.position.set(target.x + m.dir[0] * 300, m.dir[2] * 300, target.z - m.dir[1] * 300);
    if (draw) render();
  }

  // Drag to turn, wheel or pinch to zoom, arrow keys and +/- from the keyboard.
  let drag = null;
  const pointers = new Map();
  container.addEventListener('pointerdown', (e) => {
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    container.setPointerCapture(e.pointerId);
    drag = { x: e.clientX, y: e.clientY, yaw: orbit.yaw, pitch: orbit.pitch };
  });
  container.addEventListener('pointermove', (e) => {
    if (!pointers.has(e.pointerId)) return;
    const prev = pointers.get(e.pointerId);
    pointers.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.size === 2) {
      const other = [...pointers.entries()].find(([id]) => id !== e.pointerId)[1];
      const before = Math.hypot(other.x - prev.x, other.y - prev.y);
      const now = Math.hypot(other.x - e.clientX, other.y - e.clientY);
      if (before > 0 && now > 0) zoom(before / now);
      return;
    }
    if (!drag) return;
    orbit.yaw = drag.yaw - (e.clientX - drag.x) * 0.008;
    orbit.pitch = Math.max(0.12, Math.min(1.45, drag.pitch + (e.clientY - drag.y) * 0.006));
    placeCamera();
    render();
  });
  const end = (e) => { pointers.delete(e.pointerId); if (!pointers.size) drag = null; };
  container.addEventListener('pointerup', end);
  container.addEventListener('pointercancel', end);
  function zoom(f) {
    if (!box) return;
    orbit.dist = Math.max(box.size * 0.35, Math.min(box.size * 3, orbit.dist * f));
    placeCamera();
    render();
  }
  container.addEventListener('wheel', (e) => { e.preventDefault(); zoom(e.deltaY > 0 ? 1.1 : 0.9); }, { passive: false });
  container.addEventListener('keydown', (e) => {
    const k = e.key;
    if (k === 'ArrowLeft') orbit.yaw += 0.1;
    else if (k === 'ArrowRight') orbit.yaw -= 0.1;
    else if (k === 'ArrowUp') orbit.pitch = Math.min(1.45, orbit.pitch + 0.08);
    else if (k === 'ArrowDown') orbit.pitch = Math.max(0.12, orbit.pitch - 0.08);
    else if (k === '+' || k === '=') return zoom(0.9);
    else if (k === '-') return zoom(1.1);
    else return;
    e.preventDefault();
    placeCamera();
    render();
  });
  new ResizeObserver(resize).observe(container);

  return { kind: 'webgl', rebuild, recolour, retree, relight };
}

// ---------- Flat fallback: a map from above ----------
function flat(container, state) {
  const canvas = document.createElement('canvas');
  container.append(canvas);
  const ctx = canvas.getContext('2d');

  function draw() {
    const r = state.result;
    if (!r || !ctx) return;
    const w = Math.max(1, container.clientWidth), h = Math.max(1, container.clientHeight);
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const b = extent(r);
    const scale = Math.min(w / (b.x1 - b.x0 + 10), h / (b.y1 - b.y0 + 10));
    const X = (x) => w / 2 + (x - b.cx) * scale;
    const Y = (y) => h / 2 - (y - b.cy) * scale;
    ctx.fillStyle = '#8f9a96';
    ctx.fillRect(0, 0, w, h);
    ctx.fillStyle = '#c8b494';
    for (const bd of r.scene.buildings) ctx.fillRect(X(bd.x), Y(bd.y + bd.d), bd.w * scale, bd.d * scale);
    const values = state.mode === 'before' ? r.cellBefore : r.cellAfter;
    let max = 0;
    for (const v of r.cellBefore) max = Math.max(max, v);
    const s = r.cellSize * scale;
    for (let i = 0; i < values.length; i++) {
      ctx.fillStyle = `#${rampColour(max > 0 ? values[i] / max : 0).getHexString()}`;
      ctx.fillRect(X(r.cellX[i]) - s / 2, Y(r.cellY[i]) - s / 2, s, s);
    }
    for (const t of r.trees) {
      if (!t.existing && state.mode === 'before') continue;
      const c = crownAt(t, t.existing ? 0 : state.year);
      ctx.globalAlpha = 0.8;
      ctx.fillStyle = SPECIES_COLOURS[t.species] ?? '#2e6a4c';
      ctx.beginPath();
      ctx.arc(X(t.x), Y(t.y), Math.max(2, c.r * scale), 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;
    }
  }
  new ResizeObserver(draw).observe(container);
  return { kind: 'flat', rebuild: draw, recolour: draw, retree: draw, relight: () => {} };
}
