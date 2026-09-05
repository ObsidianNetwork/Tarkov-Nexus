import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { build } from 'astro';

const repo = 'https://github.com/ObsidianNetwork/Tarkov-Nexus';
const api = 'https://api.github.com/repos/ObsidianNetwork/Tarkov-Nexus/releases';

function release(tag, published, options = {}) {
  return {
    tag_name: tag,
    published_at: published,
    prerelease: tag.includes('-'),
    draft: false,
    html_url: `${repo}/releases/tag/${tag}`,
    assets: [{
      name: `TarkovNexus-${tag}-Windows-x64.zip`,
      browser_download_url: `${repo}/releases/download/${tag}/TarkovNexus-${tag}-Windows-x64.zip`,
    }],
    ...options,
  };
}

test('the built beta card downloads the newest published beta rather than a hardcoded tag', async (t) => {
  const newestBeta = release('v9.9.0-beta.2', '2026-09-05T06:37:22Z');
  const stable = release('v9.8.0', '2026-09-06T06:37:22Z');
  // Deliberately not sorted; newer stable, RC, and draft releases are not betas.
  const releases = [
    release('v9.9.0-beta.1', '2026-09-02T06:37:22Z'),
    stable,
    release('v9.9.0-rc.1', '2026-09-07T06:37:22Z'),
    release('v9.9.0-beta.3', '2026-09-08T06:37:22Z', { draft: true }),
    newestBeta,
  ];
  const requested = [];
  t.mock.method(globalThis, 'fetch', async (url) => {
    requested.push(String(url));
    if (String(url) === `${api}/latest`) return Response.json(stable);
    if (String(url) === `${api}?per_page=100`) return Response.json(releases);
    throw new Error(`Unexpected build request: ${url}`);
  });
  const output = await mkdtemp(fileURLToPath(new URL('../.beta-test-', import.meta.url)));
  t.after(() => rm(output, { recursive: true, force: true }));

  await build({
    root: new URL('../', import.meta.url),
    outDir: output,
    logLevel: 'silent',
  });
  const html = await readFile(join(output, 'index.html'), 'utf8');
  const betaLink = html.match(/<a\b[^>]*data-channel="beta"[^>]*>/)?.[0];
  assert.ok(betaLink, 'the beta download link must be rendered');
  assert.equal(betaLink.match(/href="([^"]+)"/)?.[1], newestBeta.assets[0].browser_download_url);
  assert.match(html, new RegExp(`Beta Version: <span>${newestBeta.tag_name.replaceAll('.', '\\.')}</span>`));
  assert.ok(requested.includes(`${api}?per_page=100`), 'build must fetch the release list');
  const stableLink = html.match(/<a\b[^>]*data-channel="stable"[^>]*>/)?.[0];
  assert.equal(stableLink?.match(/href="([^"]+)"/)?.[1], stable.assets[0].browser_download_url);
});
