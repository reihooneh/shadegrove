# How Shadegrove works

A walkthrough so you can explain every part of it, from the sun's position to the proof at the end.

## 1. The big picture

```
 scene.json ──▶ ParseScene        strict JSON → Scene (place, date, hours, buildings, walks, sites, trees)
                   │
                   ▼
               MakeGrid           footpaths → cells (1 m, coarser if big); the day → moments with the sun up
                   │
                   ▼
               Measure            for every cell × moment: weight (beam × heat × busyness), and the
                   │              fraction of sun getting past buildings and existing trees
                   ▼
               Optimise           every (site, species) → which cell-moments its crown shades and how much;
                   │              lazy greedy picks; upper bound on the best plan
                   ▼
               Run                growth at 1–30 years, winter sun lost, crowns for drawing → Result
```

The same Go code runs natively (the `shadegrove` command line tool) and as WebAssembly in a Web Worker for the site. The page sends a JSON string and receives one back: nothing else crosses the boundary.

## 2. File tour

| File | Job |
|---|---|
| `engine/sun.go` | The sun's elevation and azimuth (NOAA equations), refraction, a clear-sky direct beam |
| `engine/species.go` | Six street-tree species, how they grow, and Beer–Lambert light through an ellipsoid crown |
| `engine/scene.go` | The scene format, strict parsing and validation, the winter twin of a scene |
| `engine/shade.go` | The footpath grid, the hot-day weighting, ray–box tests, building and tree shade |
| `engine/plan.go` | Candidates, lazy greedy with feasibility rules, the upper bound, evaluating a plan |
| `engine/run.go` | The whole run: growth, seasons, crowns per age, notes |
| `engine/report.go` | The command line's text report |
| `cmd/shadegrove` | The command line tool |
| `cmd/wasm` | The WebAssembly entry point: `shadegrove.plan(json) → json` |
| `site/assets/js/worker.js` | Loads the engine in a Web Worker and answers requests |
| `site/assets/js/street3d.js` | The three.js street (and a flat 2D fallback): buildings, crowns, heat map, sun and shadows, orbit controls |
| `site/assets/js/{home,plan,grow,method}.js` | One script per page |
| `scripts/build.mjs` | Assembles the pages and vendored files, writes the CSP into every page |
| `tests/web/site.test.mjs` | Playwright: three sizes, overlaps, CSP, interactions, hostile inputs |

## 3. Key concepts, simply

### Where the sun is
From the date and time, compute how far the Earth is round its orbit (the sun's ecliptic longitude) and the tilt of its axis. Together they give the **declination**, how far north or south of the equator the sun is overhead, and the **equation of time**, how far sundial time runs ahead of or behind clock time. With the place's longitude and time zone, that gives the **hour angle**, and spherical trigonometry turns latitude, declination and hour angle into the sun's height and compass direction. The engine is checked against hand geometry: at noon, height = 90° − |latitude − declination|.

### Shadow by ray tracing
For each cell and moment, a ray goes from shoulder height (1.5 m) towards the sun. A building is a box, and the **slab method** clips the ray against each pair of parallel faces: if the entry point is before the exit point on all three axes, the ray hits. A crown is an ellipsoid. Squash it into a unit sphere, find where the ray enters and leaves, and the chord length gives the light let through by the **Beer–Lambert law**: `through = (through at the middle) ^ (chord / diameter)`.

### What counts as "hot sun"
Each moment's weight = direct beam (from a clear-sky air-mass model) × how hot the air is (a curve peaking at 3 pm) × the minutes it covers. Each cell's weight = its area × how busy its footpath is. The quantity to reduce is

```
exposure = Σ over cells c and moments t of  weight(c) · weight(t) · fraction of sun reaching c at t
```

### Why greedy, and why it's provably good
Planting a set of trees S leaves each cell-moment with sunlight × Π (crown transmission) over the trees in S. The sun removed, f(S), is:
- **monotone:** another tree never adds sun;
- **submodular:** a tree removes less when other crowns already dim the same cell-moments, because it multiplies a smaller remaining amount.

For monotone submodular functions with a size limit, adding the best tree each round (greedy) achieves at least (1 − 1/e) ≈ 63% of the optimum (Nemhauser, Wolsey and Fisher, 1978). **Lazy evaluation** makes it fast: a tree's gain can only shrink as the plan grows, so if a stale gain recomputed at the top of a priority queue is still the best, it really is the best. On the bundled streets, about 160 gain evaluations replace over 1,500.

### The certificate
For any plan OPT of the same size: f(OPT) ≤ f(OPT ∪ S) ≤ f(S) + Σ over OPT's trees of their gain on top of S. OPT has at most one tree per site, so that sum is at most the k largest per-site best gains. Shadegrove computes this for every plan and reports f(S) divided by the bound: a floor on how close it is to the best. The tests check the bound against exhaustive search.

### Real-world rules
- Sites closer than the minimum spacing can't both be planted, nor can a site too close to an existing tree.
- A site under power lines can't take a species taller than its limit.
- No species may supply more than a set share of the new trees (urban foresters spread species so one pest or disease can't strip a street).

These rules only ever remove options as trees are chosen, so a candidate that becomes infeasible can be dropped from the queue for good.

### Years and seasons
Height approaches the mature height exponentially (half the remaining growth every `halfYears`), and spread scales with height. Crowns are lifted to keep a 2.4 m clearance over footpaths. The same plan is evaluated at 1 to 30 years, and on a winter day six months later with deciduous trees bare, to show the trade-off between summer shade and winter sun.

## 4. Trade-offs

- **Blocks and ellipsoids, not meshes:** fast, simple and good enough for footpath-scale shade. Real canopies are clumpy, and real roofs have pitches.
- **Direct sun only:** this captures the biggest lever on how hot a walker feels, but not reflected or long-wave heat. A full comfort index (UTCI) is on the roadmap.
- **Greedy, not exact:** exact optimisation (integer programming) would need a solver and could be slow in a browser. Greedy is fast, near-optimal in practice, and comes with a proof.
- **Go to WebAssembly:** one engine for the CLI, the tests and the site. The cost is a ~4 MB download (about 1 MB compressed), loaded once, in a worker.
- **Static site, meta-tag CSP:** no server to attack or maintain. GitHub Pages can't set headers, so `frame-ancestors` isn't enforced; SECURITY.md says so.

## 5. Interview questions you might get

**Q: Why is the greedy algorithm good here, and when wouldn't it be?**
A: Removed sun is monotone and submodular: each crown multiplies the light reaching a cell, so overlapping crowns have diminishing returns. With a size limit, greedy is within 1 − 1/e of optimal. It would be worse with constraints that interact badly, such as a dollar budget with very different tree prices (that needs a cost-benefit variant) or effects that aren't submodular (trees that only help in pairs).

**Q: How do you know the plan is any good on *this* street?**
A: Two ways. Each run computes a data-dependent upper bound on the best possible plan, from submodularity, and reports the plan as a share of it. The tests also compare against exhaustive search on 40 random small streets: never below 63%, the very best on 32 of them, and the bound always above the true best.

**Q: What's lazy greedy, and why is it exact?**
A: Keep every candidate in a max-heap by its last computed gain. Pop the top; if its gain was computed for the current plan, take it; otherwise recompute and push it back. Gains only shrink as the plan grows, so a freshly recomputed gain that still beats every stale upper bound is truly the maximum. The test checks that it picks exactly what plain greedy picks.

**Q: How did you keep the browser safe from hostile scene files?**
A: The engine parses into fixed structs with unknown fields refused, a 256 KB limit, range checks and a hard cap on work. Errors never echo input. Pages only insert text with `textContent`. URL fragments are whitelisted. The CSP forbids inline and third-party code. The browser tests load a file with an HTML payload in its name and check that nothing is injected.

**Q: Why run the engine in a Web Worker?**
A: A plan takes from a fraction of a second up to a few seconds in WebAssembly. On the main thread that would freeze scrolling and the 3D view. In a worker the page stays responsive, and the worker boundary is a natural trust boundary: only strings cross it.

## 6. Ideas to extend it

1. **Open data streets:** generate scenes from OpenStreetMap buildings, footpaths and tree inventories for any address.
2. **Budget in dollars:** different prices per species and site, using the cost-benefit greedy variant (with its own guarantee).
3. **Thermal comfort:** combine shade with wall reflection and surface temperatures to score with UTCI, and plan for the hottest hour rather than the whole day.
