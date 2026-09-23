import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { after, before, test } from 'node:test';
import { chromium } from 'playwright';

let browser;
let server;
let frameServer;
let origin;
let frameOrigin;

async function listen(server) {
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return `http://127.0.0.1:${server.address().port}`;
}

before(async () => {
  const html = await readFile(new URL('../../mapwindow_assets/index.html', import.meta.url), 'utf8');
  server = createServer((_req, res) => {
    res.setHeader('Content-Type', 'text/html');
    res.end(html);
  });
  frameServer = createServer((_req, res) => {
    res.setHeader('Content-Type', 'text/html');
    res.end('<body style="margin:0;background:#182333;color:white"><button style="margin:100px">Map surface</button></body>');
  });
  origin = await listen(server);
  frameOrigin = await listen(frameServer);
  browser = await chromium.launch({ headless: process.env.HEADED !== '1' });
});

after(async () => {
  await browser?.close();
  await Promise.all([server, frameServer].map(s => s && new Promise(resolve => s.close(resolve))));
});

async function openWindow(t, width = 500) {
  const page = await browser.newPage({ viewport: { width, height: 500 } });
  page.setDefaultTimeout(3000);
  t.after(() => page.close());
  const heldStarted = Promise.withResolvers();
  const backend = {
    state: { shape: 'square', opacity: 100, sequence: 0 },
    failOpacity: null,
    holdNext: false,
    releaseHeld: () => {},
    heldStarted: heldStarted.promise,
    resizeCalls: [],
  };
  await page.exposeFunction('readAppearance', () => ({ ...backend.state }));
  await page.exposeFunction('resizeCircle', (bounds, scale) => { backend.resizeCalls.push({ bounds, scale }); });
  await page.exposeFunction('writeAppearance', async (next, sequence) => {
    if (backend.holdNext) {
      backend.holdNext = false;
      heldStarted.resolve();
      await new Promise(resolve => { backend.releaseHeld = resolve; });
    }
    if (next.opacity === backend.failOpacity) throw new Error('temporary storage failure');
    if (sequence >= backend.state.sequence) {
      backend.state = { shape: next.shape, opacity: next.opacity, sequence };
    }
    return { ...backend.state };
  });
  await page.addInitScript(() => {
    window.go = { main: { MapWindowApp: {
      GetPinned: async () => false,
      GetAppearance: () => window.readAppearance(),
      SetAppearance: (next, sequence) => window.writeAppearance(next, sequence),
      GetResizeBounds: async () => ({ x: 100, y: 120, w: innerWidth * 1.25, h: innerHeight * 1.25 }),
      ResizeCircle: (bounds, scale) => window.resizeCircle(bounds, scale),
    } } };
  });
  await page.goto(origin);
  await page.waitForFunction(() => !document.getElementById('shape-btn').disabled);
  await page.locator('#map-frame').evaluate((frame, url) => { frame.src = url; }, frameOrigin);
  return { page, backend };
}

test('appearance changes still save after the control pill reloads', async t => {
  const { page, backend } = await openWindow(t);
  for (const shape of ['circle', 'square', 'circle']) {
    await page.locator('#shape-btn').click();
    await page.waitForFunction(expected => confirmed.shape === expected, shape);
  }
  await page.reload();
  await page.waitForFunction(() => !document.getElementById('shape-btn').disabled);
  await page.locator('#shape-btn').click();
  await page.waitForFunction(() => confirmed.shape === 'square');
  assert.equal(backend.state.shape, 'square');
});

test('failed appearance save restores the last successful overlapping change', async t => {
  const { page, backend } = await openWindow(t);
  backend.holdNext = true;
  backend.failOpacity = 20;
  await page.locator('#shape-btn').click();
  await backend.heldStarted;
  await page.locator('#opacity-btn').click();
  await page.locator('#opacity-range').press('Home');
  backend.releaseHeld();
  await page.waitForFunction(() => current.shape === 'circle' && current.opacity === 100);
  assert.equal(backend.state.shape, 'circle');
  assert.equal(backend.state.opacity, 100);
});

test('opacity popover fits the supported 200-pixel minimum window', async t => {
  const { page } = await openWindow(t, 200);
  await page.locator('#opacity-btn').click();
  const bounds = await page.locator('#opacity-pop').boundingBox();
  assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 200,
    `opacity controls overflow the window: ${JSON.stringify(bounds)}`);
});

test('finishing a shape save preserves an active opacity drag', async t => {
  const { page, backend } = await openWindow(t);
  backend.holdNext = true;
  await page.locator('#shape-btn').click();
  await backend.heldStarted;
  await page.locator('#opacity-btn').click();
  const range = page.locator('#opacity-range');
  const bounds = await range.boundingBox();
  await page.mouse.move(bounds.x + bounds.width * 0.375, bounds.y + bounds.height / 2);
  await page.mouse.down();
  await page.waitForFunction(() => document.getElementById('opacity-range').value === '50');
  backend.releaseHeld();
  await page.waitForFunction(() => confirmed.shape === 'circle');
  assert.equal(await range.inputValue(), '50');
  assert.equal(await page.locator('#content').evaluate(el => el.style.getPropertyValue('--map-opacity')), '0.5');
  await page.mouse.up();
  await page.waitForFunction(() => confirmed.opacity === 50);
});

test('clicking the cross-origin map closes the opacity popover', async t => {
  const { page } = await openWindow(t);
  await page.locator('#opacity-btn').click();
  await page.frameLocator('#map-frame').getByRole('button', { name: 'Map surface' }).click();
  await page.locator('#opacity-pop').waitFor({ state: 'hidden' });
});

test('the visible circle edge resizes wide and tall windows from either side', async t => {
  const { page, backend } = await openWindow(t);
  await page.locator('#shape-btn').click();
  for (const viewport of [{ width: 640, height: 400 }, { width: 400, height: 640 }]) {
    await page.setViewportSize(viewport);
    await page.waitForFunction(width => document.getElementById('circle-resize').getAttribute('cx') === String(width / 2), viewport.width);
    const radius = Math.min(viewport.width, viewport.height) / 2 - 6;
    const start = { x: viewport.width / 2 + radius - 2, y: viewport.height / 2 };
    assert.equal(await page.evaluate(({ x, y }) => document.elementFromPoint(x, y)?.id, start), 'circle-resize');
    const count = backend.resizeCalls.length;
    await page.mouse.move(start.x, start.y);
    await page.mouse.down();
    await page.mouse.move(start.x - 40, start.y, { steps: 4 });
    await page.mouse.up();
    await page.waitForFunction(() => !resizePending);
    assert.ok(backend.resizeCalls.length > count);
    const last = backend.resizeCalls.at(-1);
    assert.deepEqual(last.bounds, { x: 100, y: 120, w: viewport.width * 1.25, h: viewport.height * 1.25 });
    assert.ok(Math.abs(last.scale - 0.8) < 0.001, `unexpected scale ${last.scale}`);
  }
});

test('square resize bounds and the circle edge stay visible when the map is dimmed', async t => {
  const { page } = await openWindow(t);
  const square = page.locator('#square-outline');
  await square.waitFor({ state: 'visible' });
  assert.equal(await square.evaluate(el => getComputedStyle(el).borderTopWidth), '2px');
  await page.locator('#shape-btn').click();
  await square.waitFor({ state: 'hidden' });
  await page.locator('#opacity-btn').click();
  await page.locator('#opacity-range').press('Home');
  assert.equal(await page.locator('#resize-frame').evaluate(el => getComputedStyle(el).opacity), '1');
  assert.equal(await page.locator('#circle-outline').evaluate(el => getComputedStyle(el).strokeWidth), '2px');
});

test('a quick edge drag still commits after delayed native bounds arrive', async t => {
  const { page, backend } = await openWindow(t);
  await page.locator('#shape-btn').click();
  await page.evaluate(() => {
    const api = window.go.main.MapWindowApp;
    const read = api.GetResizeBounds;
    api.GetResizeBounds = () => new Promise(resolve => { window.releaseBounds = async () => resolve(await read()); });
  });
  await page.mouse.move(492, 250);
  await page.mouse.down();
  await page.mouse.move(442, 250);
  await page.mouse.up();
  assert.equal(backend.resizeCalls.length, 0);
  await page.evaluate(() => window.releaseBounds());
  await page.waitForFunction(() => !resizePending);
  assert.ok(Math.abs(backend.resizeCalls.at(-1).scale - 0.8) < 0.001);
});

test('Escape restores initial bounds after an in-flight circle resize', async t => {
  const { page, backend } = await openWindow(t);
  await page.locator('#shape-btn').click();
  await page.evaluate(() => {
    const api = window.go.main.MapWindowApp;
    const resize = api.ResizeCircle;
    let held = false;
    api.ResizeCircle = (bounds, scale) => {
      if (held) return resize(bounds, scale);
      held = true;
      return new Promise(resolve => { window.releaseResize = async () => resolve(await resize(bounds, scale)); });
    };
  });
  await page.mouse.move(492, 250);
  await page.mouse.down();
  await page.mouse.move(472, 250);
  await page.waitForFunction(() => typeof window.releaseResize === 'function');
  await page.mouse.move(442, 250);
  await page.keyboard.press('Escape');
  await page.mouse.up();
  await page.evaluate(() => window.releaseResize());
  await page.waitForFunction(() => !resizePending);
  assert.equal(backend.resizeCalls.length, 2);
  assert.equal(backend.resizeCalls.at(-1).scale, 1);
});

test('a failed native bounds read releases the handle for keyboard resizing', async t => {
  const { page, backend } = await openWindow(t);
  await page.locator('#shape-btn').click();
  await page.evaluate(() => {
    const api = window.go.main.MapWindowApp;
    const read = api.GetResizeBounds;
    api.GetResizeBounds = async () => { api.GetResizeBounds = read; throw new Error('temporary native read error'); };
  });
  await page.mouse.move(492, 250);
  await page.mouse.down();
  await page.mouse.up();
  await page.waitForFunction(() => !resizePending);
  await page.locator('#circle-resize').focus();
  await page.keyboard.press('ArrowRight');
  await page.waitForFunction(() => !resizePending);
  assert.equal(backend.resizeCalls.length, 1);
  assert.ok(Math.abs(backend.resizeCalls[0].scale - 1.02) < 0.001);
});

test('keyboard navigation reaches circle resizing before entering the map', async t => {
  const { page } = await openWindow(t);
  await page.locator('#shape-btn').click();
  await page.getByRole('button', { name: 'Close', exact: true }).focus();
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(() => document.activeElement?.id), 'circle-resize');
});
