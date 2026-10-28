// Small helpers shared by the pages. Elements are built with textContent only: no HTML strings.

export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k === 'text') node.textContent = v;
    else node.setAttribute(k, v === true ? '' : String(v));
  }
  for (const c of children) if (c !== null && c !== undefined) node.append(c);
  return node;
}

export function svg(tag, attrs = {}, ...children) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'text') node.textContent = v;
    else node.setAttribute(k, String(v));
  }
  for (const c of children) if (c) node.append(c);
  return node;
}

export const pct = (v, digits = 0) => `${(v * 100).toFixed(digits)}%`;

export function clock(hour) {
  const m = Math.round(hour * 60);
  const h = Math.floor(m / 60) % 24;
  const mm = String(m % 60).padStart(2, '0');
  const suffix = h < 12 ? 'am' : 'pm';
  return `${((h + 11) % 12) + 1}:${mm} ${suffix}`;
}

export function compass(deg) {
  const p = ['north', 'north-east', 'east', 'south-east', 'south', 'south-west', 'west', 'north-west'];
  return p[Math.round((((deg % 360) + 360) % 360) / 45) % 8];
}

/** Species colours used in the 3D view, the tables and the charts. */
export const SPECIES_COLOURS = {
  'brush-box': '#2e6a4c',
  plane: '#5d8f3a',
  'chinese-elm': '#7aa35a',
  jacaranda: '#7f6ab5',
  'water-gum': '#3f8a76',
  'crepe-myrtle': '#c0577f',
};

export function speciesName(result, id) {
  return result.species.find((s) => s.id === id)?.name ?? id;
}

/** Offers a file to download, built in memory (no server involved). */
export function download(name, type, text) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = el('a', { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;
