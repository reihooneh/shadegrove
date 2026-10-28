# Security policy

## Reporting a problem

Please open a private security advisory on this repository (Security → Advisories → Report a vulnerability), or open an issue that says only that you have a security report, without the details. I aim to reply within a week.

## What Shadegrove is

A static website (HTML, CSS, JavaScript, a WebAssembly engine) served from GitHub Pages, and a command line tool that reads a JSON file. There are no accounts, no server-side code, no cookies, no analytics and no network requests beyond the site's own files. Scenes you load are read in your browser and never uploaded.

## Threat model

| What an attacker might try | How Shadegrove defends |
|---|---|
| A crafted scene file that injects HTML or script into the page (XSS) | Pages never build HTML from strings: every value goes in with `textContent`. The engine refuses names and ids outside a small character set, and its error messages never echo input. A strict CSP blocks inline scripts and styles anyway. |
| A crafted link (`plan.html#scene=…`) that loads an arbitrary file or injects values | Fragment values are whitelisted: scene ids must be one of the bundled three, numbers must match a short pattern and a range, and everything else is ignored. Scene files are only fetched from the site's own `assets/scenes/`. |
| A huge or deeply nested JSON file, or numbers like `1e999`, to crash or hang the engine | 256 KB limit, a single object with no unknown fields, number ranges, and caps on buildings, walks, sites and trees. The footpath grid coarsens automatically, and a hard cap on shade records refuses oversized scenes with a message. The engine runs in a Web Worker, and a recovered panic becomes an error message. |
| Prototype pollution through `__proto__` keys | The engine decodes into fixed Go structs and refuses unknown fields. The page copies scenes with `structuredClone` and object spread, which create own properties and never touch prototypes. |
| Path tricks in the command line tool (folders, devices, `../`) | It opens only regular files given on the command line, reads through a size limit, and never writes files. |
| A compromised dependency or CDN | Three pinned npm packages (three.js, two font packages; Playwright only for tests), all self-hosted. There is no CDN, and `npm audit` runs in CI. The CSP allows scripts only from the site itself. |
| Tampering with the deployed site | It is deployed only from `main` by GitHub Actions, with least-privilege workflow permissions (read-only by default; the deploy job alone may write to Pages). |

## Known limits

- GitHub Pages can't send HTTP security headers, so the CSP is set with a `<meta>` tag. That covers scripts, styles, workers, fonts and connections, but not `frame-ancestors`, so the site could be framed by another page. It holds no secrets or actions worth clickjacking.
- `'wasm-unsafe-eval'` is needed to run WebAssembly. It allows compiling WebAssembly, not JavaScript `eval`.
- No software is perfectly secure. This project follows common professional practice and is tested against the attacks above; please report anything it missed.
