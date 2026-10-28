/* The planner runs here, off the page's main thread, so the page never freezes while it works.
   It loads Go's WebAssembly support and the engine once, then answers one request at a time.
   Requests and replies are plain strings: nothing else crosses into the engine. */
/* global Go */
'use strict';
importScripts('../vendor/wasm_exec.js');

let ready = null;
function start() {
  if (!ready) {
    ready = (async () => {
      const go = new Go();
      const { instance } = await WebAssembly.instantiateStreaming(fetch('../shadegrove.wasm'), go.importObject);
      go.run(instance); // runs main(), which registers self.shadegrove and then waits
      if (!self.shadegrove || !self.shadegrove.ready) throw new Error('engine did not start');
    })();
  }
  return ready;
}

self.onmessage = async (event) => {
  const data = event.data;
  if (!data || typeof data.id !== 'number' || typeof data.scene !== 'string' || data.scene.length > 262144) return;
  let result;
  try {
    await start();
    result = self.shadegrove.plan(data.scene);
  } catch {
    result = JSON.stringify({ error: 'The planner could not start in this browser.' });
  }
  self.postMessage({ id: data.id, result: String(result) });
};
