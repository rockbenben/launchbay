import fs from 'node:fs';
import path from 'node:path';
import assert from 'node:assert/strict';
import { connect, seedFixture, waitFor } from './harness.mjs';

const srv = `http://127.0.0.1:${process.argv[2]}`;
const admin = process.env.LB_UI_ADMIN, token = process.env.LB_UI_WRITE;
await seedFixture(srv, admin, token);
const tagged = await fetch(srv + '/api/v1/batch', { method: 'POST',
  headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' },
  body: JSON.stringify({ profile: 'dad', by: 'web', ops: [{ op: 'set', path: 'groups/tiles-1/items/it-tiles-1-1/tags', value: ['常用', '工具'] }] }) });
assert.equal(tagged.status, 200, await tagged.text());
const c = await connect(process.argv[4]);
const { send, run, ev, sleep } = c;
const post = async (url, body) => {
  const r = await fetch(srv + url, { method: 'POST', headers: { Authorization: 'Bearer ' + admin, 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  assert.equal(r.status, 201, await r.text());
};
const home = async (profile = 'dad') => {
  await send('Page.navigate', { url: srv + '/healthz' });
  await sleep(150);
  await run(`fetch('/pin',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'profile=${profile}'}) .then(r=>r.status)`);
  await send('Page.navigate', { url: srv + '/' });
  await waitFor(c, `!!document.getElementById('lb-add')`, 8000);
};
const screenshot = async name => {
  if (!process.env.LB_UI_SHOTS) return;
  fs.mkdirSync(process.env.LB_UI_SHOTS, { recursive: true });
  const shot = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
  fs.writeFileSync(path.join(process.env.LB_UI_SHOTS, name + '.png'), Buffer.from(shot.result.data, 'base64'));
};
try {
  await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 850, deviceScaleFactor: 1, mobile: false });
  await home();
  assert.equal(await run(`!!document.querySelector('.usage-hint')`), true);
  await screenshot('desktop-hint');
  // Both settings surfaces share a pane: spacing must separate the actual forms,
  // not just the outer container that holds the hidden panes.
  const checkSettingsSpacing = async () => {
    const gap = await run(`(()=>{const rows=[...document.querySelectorAll('#prefs-pane-look > .prefs-row')];return rows[1].getBoundingClientRect().top-rows[0].getBoundingClientRect().bottom})()`);
    assert.ok(gap >= 10, `Settings forms touch: gap=${gap}`);
  };
  await ev(`document.getElementById('lb-prefs-open').click()`);
  await waitFor(c, `!!document.querySelector('#lb-prefs #prefs-pane-look > .prefs-row')`, 8000);
  await checkSettingsSpacing();
  await screenshot('settings-desktop');
  const languages = await run(`Array.from(document.querySelector('#lb-prefs-language').options, o => o.value).filter(Boolean)`);
  assert.equal(languages.length, 18);
  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
  for (const lang of languages) {
    const catalog = JSON.parse(fs.readFileSync(new URL(`../../internal/i18n/messages/${lang}.json`, import.meta.url), 'utf8'));
    const htmlLang = ({ zh: 'zh-CN', zh_TW: 'zh-TW', pt_BR: 'pt-BR' })[lang] ?? lang;
    if (await run('document.documentElement.lang') !== htmlLang) {
      await ev(`const select = document.querySelector('#lb-prefs-language');
        select.value = ${JSON.stringify(lang)};
        select.form.querySelector('button[type=submit]').click();`);
      await waitFor(c, `document.documentElement.lang === ${JSON.stringify(htmlLang)}`, 8000);
      await waitFor(c, `!!document.querySelector('#lb-sheet-h')`, 8000);
      await ev(`document.getElementById('lb-prefs-open').click()`);
      await waitFor(c, `!!document.querySelector('#lb-prefs-language')`, 8000);
    }
    assert.equal(await run(`document.getElementById('lb-prefs-open').textContent`), catalog['prefs.open.label'], lang);
    assert.equal(await run(`document.getElementById('lb-prefs-h').textContent`), catalog['prefs.heading'], lang);
    assert.equal(await run(`document.getElementById('lb-sheet-h').textContent`), catalog['app.sheetTitle'], lang);
    assert.equal(await run(`document.querySelector('#lb-prefs .close').getAttribute('aria-label')`), catalog['app.close'], lang);
    assert.equal(await run(`document.querySelector('.cmdk[role=dialog]').getAttribute('aria-label')`), catalog['app.cmdk.aria'], lang);
    assert.equal(await run(`document.documentElement.dir`), lang === 'ar' ? 'rtl' : '', lang);
    assert.equal(await run(`(() => {
      const panes = document.querySelector('#lb-prefs .prefs-panes');
      return panes.scrollWidth === panes.clientWidth;
    })()`), true, `${lang} settings overflow on a 390px viewport`);
    if (['ar', 'pt_BR', 'vi'].includes(lang)) await screenshot(`settings-mobile-${lang}`);
  }
  await ev(`document.querySelector('#lb-prefs button[name=theme][value=light]').click()`);
  await waitFor(c, `document.documentElement.dataset.theme === 'light'`, 8000);
  await run(`fetch('/admin/login', {
    method: 'POST', body: new URLSearchParams({key: ${JSON.stringify(admin)}})
  }).then(r => r.status)`);
  await send('Page.navigate', { url: srv + '/admin' });
  await waitFor(c, `!!document.getElementById('lb-admin-language')`, 8000);
  assert.equal(await run(`document.documentElement.lang`), 'vi');
  assert.equal(await run(`document.documentElement.dataset.theme`), 'light');
  assert.equal(await run(`document.getElementById('lb-admin-language').options.length`), 19);
  assert.equal(await run(`(() => {
    const nav = document.querySelector('.adm-bar nav');
    const logout = nav.querySelector('button');
    return nav.scrollWidth === nav.clientWidth && logout.scrollWidth === logout.clientWidth;
  })()`), true, 'long administration actions fit on a narrow screen');
  await screenshot('admin-mobile-vi');
  await run(`fetch('/admin/logout', {method: 'POST'}).then(r => r.status)`);
  await home();
  await waitFor(c, `!!document.getElementById('lb-sheet-h')`, 8000);
  await ev(`document.getElementById('lb-prefs-open').click()`);
  await waitFor(c, `!!document.querySelector('#lb-prefs button[name=theme][value=auto]')`, 8000);
  await ev(`document.querySelector('#lb-prefs button[name=theme][value=auto]').click()`);
  await waitFor(c, `!document.documentElement.hasAttribute('data-theme')`, 8000);
  console.log('PASS management inherits the active language and theme; system theme removes the explicit override');
  await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 850, deviceScaleFactor: 1, mobile: false });
  await send('Page.navigate', { url: srv + '/prefs' });
  await waitFor(c, `!!document.querySelector('#prefs-pane-look > .prefs-row')`, 8000);
  await checkSettingsSpacing();
  assert.equal(await run(`document.querySelector('#lb-prefs-language').value`), languages.at(-1));
  await ev(`const select = document.querySelector('#lb-prefs-language');
    select.value = '';
    select.form.querySelector('button[type=submit]').click();`);
  await waitFor(c, `location.pathname === '/' && document.documentElement.lang === 'zh-CN'`, 8000);
  console.log('PASS all 18 languages render the main page, settings, add dialog and command palette; standalone settings restores the default');
  await ev(`document.querySelector('.skip-link').focus()`);
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 });
  assert.equal(await run(`document.activeElement===document.querySelector('main')`), true);
  console.log('PASS settings forms remain separate in both surfaces; keyboard skip reaches main');
  await run(`fetch('/prefs',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:'theme=light'}).then(r=>r.status)`);
  await home();
  await screenshot('desktop-light');
  await ev(`document.querySelector('.usage-hint button').click()`);
  await home();
  assert.equal(await run(`!!document.querySelector('.usage-hint')`), false);
  console.log('PASS usage hint dismisses and stays dismissed after navigation');

  // Force a conflicting suggestion while preserving the real batch write.
  await ev(`window.__fetch=window.fetch;window.fetch=(url,opts)=>String(url).includes('/api/v1/resolve')?Promise.resolve(new Response(JSON.stringify({title:'Drop target',suggestedGroup:{id:'tiles-1',title:'Tiles'}}),{headers:{'Content-Type':'application/json'}})):window.__fetch(url,opts)`);
  await ev(`const dt=new DataTransfer();dt.setData('text/uri-list','https://drop.example.com/explicit');const e=new DragEvent('drop',{bubbles:true,cancelable:true,dataTransfer:dt});document.querySelector('h2[data-group="list-1"]').dispatchEvent(e)`);
  await waitFor(c, `!!document.querySelector('[data-gid="list-1"] a[href="https://drop.example.com/explicit"]')`, 8000);
  const data = await (await fetch(srv + '/api/v1/data?profile=dad', { headers: { Authorization: 'Bearer ' + token } })).json();
  assert.ok(data.data.groups.find(g => g.id === 'list-1').items.some(i => i.url === 'https://drop.example.com/explicit'));
  console.log('PASS explicit drop target wins over suggested group on screen and disk');

  await post('/api/v1/profiles', { id: 'empty-ui', title: 'New profile', nopass: true });
  await home('empty-ui');
  assert.equal(await run(`!!document.querySelector('[data-add-first]:not([hidden])') && !!document.querySelector('.empty-actions a[href="/import"]')`), true);
  await screenshot('empty-start');
  await ev(`document.querySelector('[data-add-first]').click()`);
  assert.equal(await run(`!document.getElementById('lb-sheet').hidden`), true);
  console.log('PASS empty profile offers add and import; add opens the existing sheet');

  await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
  await send('Emulation.setTouchEmulationEnabled', { enabled: true });
  await send('Emulation.setEmulatedMedia', { features: [{ name: 'pointer', value: 'coarse' }, { name: 'hover', value: 'none' }] });
  await home();
  await ev(`localStorage.removeItem('lb-usage-hint-v1-dad')`);
  await home();
  assert.equal(await run(`matchMedia('(pointer:coarse)').matches`), true);
  assert.equal(await run(`getComputedStyle(document.querySelector('.usage-touch')).display!=='none'`), true);
  await screenshot('mobile-profile');
  await ev(`document.getElementById('lb-prefs-open').click()`);
  await waitFor(c, `!!document.querySelector('#lb-prefs #prefs-pane-look > .prefs-row')`, 8000);
  await screenshot('settings-mobile');
  const settings = await run(`(() => {
    const dialog = document.getElementById('lb-prefs');
    const panes = dialog.querySelector('.prefs-panes');
    const top = selector => dialog.querySelector(selector).getBoundingClientRect().top;
    const closeTop = top('.close');
    const navTop = top('.prefs-nav');
    panes.scrollTop = panes.scrollHeight;
    const light = getComputedStyle(panes).scrollbarColor;
    document.documentElement.dataset.theme = 'dark';
    const dark = getComputedStyle(panes).scrollbarColor;
    document.documentElement.dataset.theme = 'light';
    return {
      scrolled: panes.scrollTop > 0,
      closeFixed: top('.close') === closeTop,
      navFixed: top('.prefs-nav') === navTop,
      fitsWidth: panes.scrollWidth === panes.clientWidth,
      themed: light !== 'auto' && dark !== 'auto' && light !== dark,
    };
  })()`);
  assert.deepEqual(settings, { scrolled: true, closeFixed: true, navFixed: true, fitsWidth: true, themed: true });
  await ev(`document.querySelector('#lb-prefs label[for="prefs-tab-groups"]').click()`);
  assert.equal(await run(`document.querySelector('#lb-prefs .prefs-panes').scrollTop`), 0);
  await ev(`document.querySelector('#lb-prefs .close').click()`);
  console.log('PASS settings scroll independently with fixed controls and theme colors');
  const point = await run(`(()=>{const r=document.querySelector('li[data-item]').getBoundingClientRect();return {x:r.left+25,y:r.top+20}})()`);
  await send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ ...point }] });
  await sleep(520);
  assert.equal(await run(`document.body.dataset.multi==='on'`), true);
  await send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  assert.equal(await run(`document.querySelectorAll('li.sel-mark').length`), 1);
  await ev(`document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`);
  await ev(`document.querySelector('#lb-banner button')?.click()`);
  await send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ ...point }] });
  await send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: point.x, y: point.y - 50 }] });
  await sleep(520);
  assert.equal(await run(`document.body.dataset.multi==='on'`), false);
  await send('Input.dispatchTouchEvent', { type: 'touchCancel', touchPoints: [] });
  console.log('PASS touch hold selects; scrolling cancels hold');

  await ev(`document.getElementById('lb-add').click();document.documentElement.style.setProperty('--lb-vh','360px')`);
  const fits = await run(`(()=>{const s=document.getElementById('lb-sheet');const r=s.getBoundingClientRect();s.scrollTop=s.scrollHeight;const b=document.querySelector('#lb-sheet button[type=submit]').getBoundingClientRect();return {top:r.top,bottom:r.bottom,scroll:s.scrollHeight>s.clientHeight,target:b.height,saveTop:b.top,saveBottom:b.bottom}})()`);
  assert.ok(fits.top >= 0 && fits.bottom <= 360 && fits.scroll && fits.target >= 44 && fits.saveTop >= fits.top && fits.saveBottom <= fits.bottom, JSON.stringify(fits));
  await screenshot('mobile-keyboard-space');
  assert.equal(c.errs.length, 0, JSON.stringify(c.errs));
  console.log('PASS shortened keyboard viewport keeps form scrollable and save target accessible');
} finally {
  await c.close();
}
