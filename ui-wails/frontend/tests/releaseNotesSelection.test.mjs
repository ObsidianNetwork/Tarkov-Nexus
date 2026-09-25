import assert from 'node:assert/strict';
import test from 'node:test';
import { offeredReleaseVersion } from '../src/features/release-notes/selection.ts';
import { parseUpdateStatus } from '../src/types/updater.ts';
const status = { currentVersion:'1.2.3', updateAvailable:true, latestVersion:'1.3.0-beta.2+build.7', checking:false, downloading:false, installing:false };
test('only the authoritative valid offer becomes a choice', () => {
 assert.equal(offeredReleaseVersion(status, '1.2.3'), '1.3.0-beta.2+build.7');
 assert.equal(offeredReleaseVersion({...status, updateAvailable:false}, '1.2.3'), null);
 assert.equal(offeredReleaseVersion({...status, latestVersion:'v1.2.3'}, '1.2.3'), null);
 for (const latestVersion of ['', 'dev', '1.2', '1.02.3', '1.3.0-beta.01', '../../latest']) {
  assert.equal(offeredReleaseVersion({...status, latestVersion}, '1.2.3'), null);
 }
});
test('the status boundary rejects wrong shapes and accepts an uninitialized updater', () => {
 assert.equal(parseUpdateStatus(null), null);
 assert.equal(parseUpdateStatus({...status, updateAvailable:'true'}), null);
 assert.equal(parseUpdateStatus({...status, latestVersion:undefined})?.latestVersion, '');
 assert.equal(parseUpdateStatus(status)?.latestVersion, status.latestVersion);
});
