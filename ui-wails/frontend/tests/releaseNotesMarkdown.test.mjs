import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { chromium } from 'playwright';
import { createServer } from 'vite';

let server;
let browser;
let origin;

const loadRendererModules = () => Promise.all([
  import('/@id/react'),
  import('/@id/react-dom/server'),
  import('/src/features/release-notes/ReleaseNotesMarkdown.tsx'),
]);

before(async () => {
  server = await createServer({ server: { host: '127.0.0.1', port: 0, open: false } });
  await server.listen();
  origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  try {
    for (let attempt = 0; attempt < 3; attempt++) {
      await page.goto(origin);
      try {
        await page.evaluate(loadRendererModules);
        break;
      } catch (error) {
        if (attempt === 2 || !String(error).includes('Execution context was destroyed')) throw error;
      }
    }
  } finally {
    await page.close();
  }
});

after(async () => {
  await browser?.close();
  await server?.close();
});

const note = {tag:'v1.2.3-beta.1+build.2',name:'Release',releaseUrl:'https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/tag/v1.2.3-beta.1%2Bbuild.2',publishedAt:null};
const render = async (...bodies) => {
 const page = await browser.newPage();
 try {
  await page.goto(origin);
  return await page.evaluate(async ({ note, bodies }) => {
   const [reactModule, serverModule, { ReleaseNotesMarkdown }] = await Promise.all([
    import('/@id/react'),
    import('/@id/react-dom/server'),
    import('/src/features/release-notes/ReleaseNotesMarkdown.tsx'),
   ]);
   const React = reactModule.default;
   const renderToStaticMarkup = serverModule.renderToStaticMarkup ?? serverModule.default.renderToStaticMarkup;
   return renderToStaticMarkup(React.createElement(React.Fragment,null,
    ...bodies.map((body,index)=>React.createElement(ReleaseNotesMarkdown,{key:index,note:{...note,body}}))));
  }, { note, bodies });
 } finally {
  await page.close();
 }
};

test('relative files resolve at the exact tag and dot segments stay in the repository',async()=>{
 const html=await render('[Guide](docs/../README.md) [Root](/ObsidianNetwork/Tarkov-Nexus/issues) [Parent](../../LICENSE)');
 assert.ok(html.includes('href="https://github.com/ObsidianNetwork/Tarkov-Nexus/blob/v1.2.3-beta.1%2Bbuild.2/README.md"'));
 assert.ok(html.includes('href="https://github.com/ObsidianNetwork/Tarkov-Nexus/issues"'));
 assert.ok(html.includes('href="https://github.com/ObsidianNetwork/Tarkov-Nexus/blob/v1.2.3-beta.1%2Bbuild.2/LICENSE"'));
});
test('relative images use raw files while other hosts remain explicit links',async()=>{
 const html=await render('![Diagram](images/map.png) ![External](https://example.com/map.png) ![Private](https://127.0.0.1:8443/action) ![Blocked](javascript:alert)');
 assert.ok(html.includes('src="https://raw.githubusercontent.com/ObsidianNetwork/Tarkov-Nexus/v1.2.3-beta.1%2Bbuild.2/images/map.png"'));
 assert.ok(html.includes('referrerPolicy="no-referrer"'));
 assert.ok(html.includes('href="https://example.com/map.png"'));
 assert.ok(html.includes('href="https://127.0.0.1:8443/action"'));
 assert.ok(!html.includes('src="https://example.com/map.png"'));
 assert.ok(!html.includes('src="https://127.0.0.1:8443/action"'));
 assert.ok(html.includes('Blocked'));
 assert.ok(!html.includes('javascript:'));
});
test('duplicate and non-ASCII headings get unique local GitHub-style IDs',async()=>{
 const html=await render('# Changes\n\n## Changes\n\n## 日本語の変更\n\n[Jump](#changes-1)');
 const ids=Array.from(html.matchAll(/<h[1-6] id="([^"]+)"/g),m=>m[1]);
 assert.equal(ids.length, 3);
 assert.match(ids[0], /^release-note-.+-changes$/);
 assert.equal(ids[1], ids[0]+'-1');
 assert.match(ids[2], /^release-note-.+-日本語の変更$/);
 assert.ok(html.includes('href="'+note.releaseUrl+'#changes-1"'));
});
test('named anchors and footnotes survive with namespaced references',async()=>{
 const html=await render('<a name="install"></a>\n\n[Install](#install)\n\nFootnote[^one].\n\n[^one]: Details.');
 assert.match(html, /id="release-note-.+-install"/);
 assert.match(html, /id="release-note-.+-user-content-fn-one"/);
 assert.match(html, /id="release-note-.+-user-content-fnref-one"/);
 assert.match(html, /aria-describedby="release-note-.+-footnote-label"/);
 assert.ok(html.includes('Footnotes'));
});
test('raw content cannot execute or add arbitrary controls and styles',async()=>{
 const html=await render('<script>alert(1)</script><style>body{display:none}</style><form><input value="secret"></form><iframe src="https://example.com"></iframe><a id="location" href="data:text/html,x" onclick="alert(1)" style="color:red">Unsafe</a><img src="http://example.com/x" srcset="https://example.com/a 2x" onerror="alert(1)" alt="Blocked HTTP">\n\n- [x] Done');
 assert.doesNotMatch(html, /<script|<style|<form|<iframe|onclick=|onerror=|srcSet=|style=|data:text|src="http:/i);
 assert.ok(!html.includes('id="location"'));
 assert.ok(html.includes('type="checkbox"'));
 assert.ok(html.includes('disabled=""'));
 assert.ok(html.includes('Blocked HTTP'));
});

test('a named anchor keeps its alias when it also has an id',async()=>{
 const html=await render('<a id="section" name="alias"></a> [Alias](#alias)');
 assert.match(html, /id="release-note-.+-section"/);
 assert.match(html, /data-note-anchor="release-note-.+-alias"/);
});
test('simultaneous readers namespace heading and footnote targets independently',async()=>{
 const body='# Same\n\nA note[^one].\n\n[^one]: Note.';
 const html=await render(body, body);
 const ids=Array.from(html.matchAll(/id="([^"]+)"/g),m=>m[1]);
 assert.equal(new Set(ids).size, ids.length);
 assert.equal(ids.filter(id=>id.endsWith('-same')).length, 2);
});

test('CJK prose receives language hints while English and code keep their own language',async()=>{
 const html=await render('地図の表示とリリースノートの読みやすさを改善しました。\n\n中文内容。\n\nEnglish guidance quoting 日本語.\n\n```text\n日本語のコード\n```\n\n<p lang="en">English with 日本語</p>');
 assert.ok(html.includes('<p lang="ja">地図'));
 assert.ok(html.includes('<p lang="zh">中文内容'));
 assert.ok(html.includes('<p>English guidance'));
 assert.ok(html.includes('<p lang="en">English with'));
 assert.ok(!html.includes('<code lang="ja"'));
});

test('large notes cap automatic image loading and retain links for omitted images',async()=>{
 const markdown=Array.from({length:40},(_,index)=>`![Diagram ${index}](https://raw.githubusercontent.com/owner/repo/main/${index}.png)`).join('\n\n');
 const html=await render(markdown);
 assert.equal((html.match(/<img /g) ?? []).length, 32);
 assert.equal((html.match(/ omitted<\/a>/g) ?? []).length, 8);
 assert.ok(html.includes('href="https://raw.githubusercontent.com/owner/repo/main/39.png"'));
});

test('an offered release treats the Go zero time as an unknown publication date',async()=>{
 const page=await browser.newPage();
 try {
  await page.goto(origin);
  const publishedAt=await page.evaluate(async()=>{
   const {offeredNotes}=await import('/src/features/release-notes/offeredNotes.ts');
   return offeredNotes({
    version:'1.3.0',releaseUrl:'https://github.com/ObsidianNetwork/Tarkov-Nexus/releases/tag/v1.3.0',
    releaseDate:'0001-01-01T00:00:00Z',releaseName:'Release',releaseBody:'# Notes',
    assetUrl:'https://example.invalid/release.zip',assetName:'release.zip',assetSize:1,isPrerelease:false,
   }).publishedAt;
  });
  assert.equal(publishedAt,null);
 } finally {
  await page.close();
 }
});
