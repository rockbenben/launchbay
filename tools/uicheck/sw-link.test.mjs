import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

function worker() {
  const buckets = new Map(), kv = new Map(), events = new Map();
  const caches = {
    async open(name) {
      if (!buckets.has(name)) buckets.set(name, new Map());
      const bucket = buckets.get(name);
      return {
        async put(req, res) { bucket.set(req.url, res); },
        async match(req) { return bucket.get(req.url)?.clone(); },
        async keys() { return [...bucket.keys()].map(url => new Request(url)); },
        async delete(req) { return bucket.delete(req.url); },
      };
    },
    async match(req, { cacheName }) { return buckets.get(cacheName)?.get(req.url)?.clone(); },
    async delete(name) { return buckets.delete(name); },
    async keys() { return [...buckets.keys()]; },
  };
  const ctx = vm.createContext({
    URL, setTimeout, clearTimeout, caches,
    fetch: async () => { throw new Error('offline'); },
    self: { location: { origin: 'http://nas' }, addEventListener: (name, fn) => events.set(name, fn) },
  });
  vm.runInContext(fs.readFileSync(new URL('../../internal/render/web/sw.js', import.meta.url), 'utf8'), ctx);
  ctx.kvGet = async (key) => kv.get(key);
  ctx.kvSet = async (key, value) => { kv.set(key, value); };
  const req = new Request('http://nas/');
  const html = (id, mode, body) => new Response(body, { headers: { 'X-LB-Profile': id, ...(mode ? { 'X-LB-Link-Mode': mode } : {}) } });
  return { ctx, caches, buckets, kv, events, req, html };
}

test('mode bucket wins, legacy and picker remain fallbacks; auxiliary pages retain mode', async () => {
  const { ctx, caches, buckets, kv, req, html } = worker();
  await ctx.storeHTML(req, html('-picker', '', 'picker'));
  await ctx.storeHTML(req, html('dad', 'lan', 'LAN'));
  await ctx.storeHTML(req, html('dad', 'wan', 'WAN'));
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'WAN');
  assert.equal(await (await (await caches.open('lb-html:dad:lan')).match(req)).text(), 'LAN');
  await ctx.storeHTML(new Request('http://nas/prefs'), html('dad', '', 'prefs'));
  assert.equal(kv.get('currentProfile').mode, 'wan');
  await ctx.storeHTML(req, html('-admin', '', 'private'));
  assert.equal(buckets.has('lb-html:-admin'), false);
  await caches.delete('lb-html:dad:wan');
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'WAN');
  await (await caches.open('lb-html:dad')).put(req, html('dad', 'lan', 'stale'));
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'picker');
  await caches.delete('lb-html:-picker');
  await assert.rejects(() => ctx.htmlFirst(req), /nothing cached/);
});

test('legacy string record and switch clear all three buckets without touching others', async () => {
  const { ctx, buckets, kv, events, req, html } = worker();
  await ctx.storeHTML(req, html('dad', 'lan', 'LAN'));
  await ctx.storeHTML(req, html('dad', 'wan', 'WAN'));
  await ctx.storeHTML(req, html('mom', 'lan', 'MOM'));
  kv.set('currentProfile', 'dad');
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'WAN');
  let pending;
  events.get('message')({ data: { type: 'switch', from: 'dad', to: '' }, waitUntil(p) { pending = p; } });
  await pending;
  for (const key of ['lb-html:dad', 'lb-html:dad:lan', 'lb-html:dad:wan']) assert.equal(buckets.has(key), false);
  assert.equal(buckets.has('lb-html:mom:lan'), true);
  assert.equal(kv.get('currentProfile'), '-picker');
  await ctx.storeHTML(req, html('', '', 'unknown'));
  assert.equal(buckets.has('lb-html:'), false);
});

test('activation separates mixed v3 buckets by response owner and mode', async () => {
  const { ctx, caches, kv, events, req, html } = worker();
  const aux = new Request('http://nas/prefs');
  const old = await caches.open('html-dad-lan');
  await old.put(req, html('dad', 'lan', 'LAN'));
  await old.put(aux, html('dad-lan', '', 'prefs'));
  await (await caches.open('html-picker')).put(req, html('picker', '', 'ambiguous'));
  let done;
  events.get('activate')({ waitUntil(p) { done = p; } });
  await done;
  kv.set('currentProfile', { id: 'dad', mode: 'lan' });
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'LAN');
  kv.set('currentProfile', 'dad-lan');
  assert.equal(await (await ctx.htmlFirst(aux)).text(), 'prefs');
  await assert.rejects(() => ctx.htmlFirst(req), /nothing cached/);
  await ctx.storeHTML(req, html('picker', 'wan', 'named profile'));
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'named profile');
  kv.set('currentProfile', '-picker');
  await assert.rejects(() => ctx.htmlFirst(req), /nothing cached/);
});
