import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { releaseNotesState, failedReleaseNotes, retryDelay } from '../src/features/release-notes/state.ts';

const version = '1.2.3';
const exact = 'https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/tag/v1.2.3';
const all = 'https://github.com/ObsidianNetwork/Tarkov-Nexus/releases';
const note = { tag: 'v1.2.3', name: 'Release', body: '', releaseUrl: exact, publishedAt: '2026-09-05T00:00:00Z' };
const response = { version, tag: 'v1.2.3', note, lookup: 'published', saved: true, releaseUrl: exact, retryAt: null };

describe('release-note outcomes', () => {
  test('empty published record stays content', () => {
    const state = releaseNotesState(response, version);
    assert.equal(state.kind, 'content');
    assert.equal(state.note.body, '');
    assert.equal(state.retryable, false);
    assert.equal(state.releaseUrl, exact);
  });
  test('invalid and zero publication dates remain unknown', () => {
    const malformed = releaseNotesState({ ...response, note: { ...note, publishedAt: 'not-a-date' } }, version);
    const zero = releaseNotesState({ ...response, note: { ...note, publishedAt: '0001-01-01T00:00:00Z' } }, version);
    assert.equal(malformed.kind, 'content');
    assert.equal(malformed.publishedAt, null);
    assert.equal(zero.kind, 'content');
    assert.equal(zero.publishedAt, null);
  });
  test('missing release retains saved content and all-releases navigation', () => {
    const state = releaseNotesState({ ...response, lookup: 'missing', releaseUrl: all }, version);
    assert.equal(state.kind, 'content');
    assert.equal(state.publication, 'unavailable');
    assert.equal(state.releaseUrl, all);
    assert.equal(state.retryable, false);
  });
  test('missing without content preserves the requested identity', () => {
    const state = releaseNotesState({ ...response, lookup: 'missing', note: null, saved: false, releaseUrl: all }, version);
    assert.equal(state.kind, 'missing');
    assert.equal(state.version, version);
    assert.equal(state.releaseUrl, all);
  });
  test('failure keeps saved empty content and manual retry timing', () => {
    const retryAt = '2026-09-25T12:00:00Z';
    const state = releaseNotesState({ ...response, lookup: 'failed', retryAt }, version);
    assert.equal(state.kind, 'content');
    assert.equal(state.note.body, '');
    assert.equal(state.publication, 'failed');
    assert.equal(state.retryable, true);
    assert.equal(state.retryAt, retryAt);
    assert.equal(state.releaseUrl, exact);
  });
  test('binding rejection retains displayed text and exact navigation', () => {
    const previous = releaseNotesState(response, version);
    const state = failedReleaseNotes(version, previous);
    assert.equal(state.kind, 'content');
    assert.equal(state.source, 'saved');
    assert.equal(state.retryable, true);
    assert.equal(state.releaseUrl, exact);
  });
  test('mismatched completion cannot relabel notes as requested version', () => {
    const state = releaseNotesState({ ...response, version: '2.0.0' }, version);
    assert.equal(state.kind, 'failed');
    assert.equal(state.version, version);
    assert.equal(state.releaseUrl, exact);
  });
  test('retry delay permits retry only after deadline and ignores malformed dates', () => {
    assert.equal(retryDelay('2026-09-25T12:00:00Z', Date.parse('2026-09-25T11:59:00Z')), 60000);
    assert.equal(retryDelay('2026-09-25T12:00:00Z', Date.parse('2026-09-25T12:00:00Z')), 0);
    assert.equal(retryDelay('invalid', 0), 0);
    assert.equal(retryDelay(null, 0), 0);
  });
});
