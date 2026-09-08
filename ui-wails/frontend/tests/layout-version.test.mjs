import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { after, before, test } from 'node:test';
import { chromium } from 'playwright';
import { createServer } from 'vite';

let server;
let browser;
let origin;

before(async () => {
  server = await createServer({
    server: { host: '127.0.0.1', port: 0, open: false },
  });
  await server.listen();
  origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({ headless: process.env.HEADED !== '1' });
});

after(async () => {
  await browser?.close();
  await server?.close();
});

const cases = [
  { version: '3.4.0-beta.1', channel: 'stable', badge: 'BETA' },
  { version: '3.4.0', channel: 'beta', badge: 'STABLE' },
  { version: '3.4.0-beta.2+build.7', channel: 'stable', badge: 'BETA' },
  { version: '3.4.0+build-beta.1', channel: 'beta', badge: 'STABLE' },
  { version: '0.0.0-dev', channel: 'beta', badge: 'STABLE' },
];

for (const fixture of cases) {
  test(`${fixture.version} shows ${fixture.badge} with ${fixture.channel} updates selected`, async (t) => {
    // Given the actual frontend, with only the native Wails boundary replaced.
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    t.after(() => page.close());
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(({ version, channel }) => {
      window.go = {
        main: {
          App: {
            GetVersion: async () => version,
            GetConfig: async () => ({
              setupComplete: true,
              updateSettings: { updateChannel: channel },
            }),
            GetStatus: async () => ({ connected: false }),
            GetUpdateStatus: async () => ({ currentVersion: version }),
            IsRunning: async () => false,
            GetLogs: async () => [],
            GetLogStats: async () => ({ total: 0, debug: 0, info: 0, warning: 0, error: 0 }),
          },
        },
      };
      window.runtime = { EventsOnMultiple: () => () => {} };
    }, fixture);

    // When the installed version has arrived through the real generated binding.
    await page.goto(`${origin}/logs`);
    const label = fixture.version === '0.0.0-dev' ? 'DEV' : `v${fixture.version}`;
    const versionLabel = page.getByText(label, { exact: true });
    await versionLabel.waitFor({ state: 'visible' });

    // Then the badge describes the installed build, not the update preference.
    const versionRow = versionLabel.locator('..');
    assert.equal(await versionRow.locator('span').last().innerText(), fixture.badge);
    assert.deepEqual(errors, []);
    if (process.env.SCREENSHOT_DIR) {
      await mkdir(process.env.SCREENSHOT_DIR, { recursive: true });
      await page.screenshot({
        path: resolve(process.env.SCREENSHOT_DIR, `${fixture.version}-${fixture.channel}.png`),
      });
    }
  });
}
