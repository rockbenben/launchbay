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
      return { async put(req, res) { bucket.set(req.url, res); }, async match(req) { return bucket.get(req.url)?.clone(); } };
    },
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
  await ctx.storeHTML(req, html('picker', '', 'picker'));
  await ctx.storeHTML(req, html('dad', 'lan', 'LAN'));
  await ctx.storeHTML(req, html('dad', 'wan', 'WAN'));
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'WAN');
  assert.equal(await (await (await caches.open('html-dad-lan')).match(req)).text(), 'LAN');
  await ctx.storeHTML(new Request('http://nas/prefs'), html('dad', '', 'prefs'));
  assert.equal(kv.get('currentProfile').mode, 'wan');
  await ctx.storeHTML(req, html('-admin', '', 'private'));
  assert.equal(buckets.has('html--admin'), false);
  await caches.delete('html-dad-wan');
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'WAN');
  await caches.delete('html-dad');
  assert.equal(await (await ctx.htmlFirst(req)).text(), 'picker');
  await caches.delete('html-picker');
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
  for (const key of ['html-dad', 'html-dad-lan', 'html-dad-wan']) assert.equal(buckets.has(key), false);
  assert.equal(buckets.has('html-mom-lan'), true);
  assert.equal(kv.get('currentProfile'), 'dad');
  await ctx.storeHTML(req, html('', '', 'unknown'));
  assert.equal(buckets.has('html-'), false);
});
