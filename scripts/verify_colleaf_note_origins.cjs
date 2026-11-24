/* Optional pinned note-source intent/save/edit qualification; no production JS. */
const assert = require('node:assert/strict');
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const {createRequire, stripTypeScriptTypes} = require('node:module');
const root = path.resolve(__dirname, '..'), qa = path.join(root, 'output/colleaf-qa');
const req = createRequire(path.join(qa, 'package.json'));
const core = req('@univerjs/core'), engine = req('@univerjs/engine-render');
const docs = req('@univerjs/docs'), drawing = req('@univerjs/drawing');
const dd = req('@univerjs/docs-drawing'), hyper = req('@univerjs/docs-hyper-link');
const {FUniver} = req('@univerjs/core/facade');
req('@univerjs/docs/facade');
for (const name of ['core', 'engine-render', 'docs', 'drawing', 'docs-drawing', 'docs-hyper-link']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
}
function records(data) {
  return data.body.customRanges.find(r => r.properties?.profile === 'manydoc.native-intent').properties.records;
}
function evidence(data) {
  return JSON.parse(records(data).find(r => r.name === 'manydoc.document-properties.v1').data).evidence;
}
function save(data) {
  const u = new core.Univer();
  try {
    for (const plugin of [docs.UniverDocsPlugin, drawing.UniverDrawingPlugin, dd.UniverDocsDrawingPlugin, hyper.UniverDocsHyperLinkPlugin]) u.registerPlugin(plugin);
    return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(data)).save()));
  } finally { u.dispose(); }
}
function inspect(data, originals, label = 'α') {
  assert.deepEqual(evidence(data).noteOrigins, originals);
  const m = new core.DocumentDataModel(data), v = new engine.DocumentViewModel(m);
  const s = new engine.DocumentSkeleton(v, new core.LocaleService());
  try {
    s.calculate();
    const glyphs = p => p.sections.flatMap(s => s.columns.flatMap(c => c.lines.flatMap(l => l.divides.flatMap(d => d.glyphGroup))));
    const pages = s.getSkeletonData().pages;
    const emoji = pages.flatMap(glyphs).find(g => g.content === '😀');
    assert.ok(emoji); assert.equal(emoji.ts.bl, 1); assert.equal(emoji.ts.fs, 13);
    const foot = pages.flatMap(p => p.notes || []).find(n => n.noteId === 'manydoc-note-1');
    assert.ok(foot);
    const mark = glyphs(foot.page).find(g => g.content === label);
    assert.ok(mark); assert.equal(mark.ts.bl, 1); assert.equal(mark.ts.fs, 13);
    assert.notEqual(mark.ts.ul?.s, 1, 'source history must not decorate body marks');
  } finally { s.dispose(); v.dispose(); m.dispose(); }
}
async function main() {
  const source = fs.readFileSync(path.join(root, 'output/codec/colleaf-format-0.txt'), 'utf8');
  assert.equal(crypto.createHash('sha256').update(source).digest('hex'), 'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
  const app = await import('data:text/javascript;base64,' + Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
  const validate = document => app.validateDocument({type: 'vibedada-document', version: 3, engine: 'univer', engineVersion: '1.0.4', document});
  const data = JSON.parse(fs.readFileSync(path.join(qa, 'note-origins.json')));
  const origins = evidence(data).noteOrigins;
  assert.equal(origins.length, 3);
  assert.deepEqual(origins.map(o => [o.nativeId, o.kind, o.id]), [
    ['manydoc-note-1', 'footnote', 'n'], ['manydoc-note-2', 'endnote', 'n'], ['manydoc-note-3', 'footnote', 'inference']
  ]);
  assert.equal(origins[0].source.element, 'foot-definition');
  assert.equal(origins[0].labelSource.element, 'foot-label');
  assert.equal(origins[0].kindInferred, true);
  assert.equal(origins[2].kindInferred, true);
  assert.equal(origins[2].source, undefined);
  for (const o of origins) {
    assert.equal(o.version, 1); assert.match(o.bodyFingerprint, /^[a-f0-9]{64}$/); assert.match(o.labelFingerprint, /^[a-f0-9]{64}$/);
    for (const key of ['text', 'body', 'blocks', 'runs', 'label', 'bodyLabel', 'bodyLabelContent']) assert.ok(!Object.hasOwn(o, key));
  }
  validate(data); inspect(data, origins);
  const saved = save(save(data)); validate(saved); inspect(saved, origins);
  assert.equal(saved.body.dataStream, data.body.dataStream);
  for (const id of Object.keys(data.notes)) assert.equal(saved.notes[id].body.dataStream, data.notes[id].body.dataStream);
  fs.writeFileSync(path.join(qa, 'note-origins-saved.json'), JSON.stringify(saved));
  const body = structuredClone(saved);
  body.notes['manydoc-note-1'].body = core.TextX.apply(body.notes['manydoc-note-1'].body, new core.TextX().retain(2).insert(1, {dataStream: 'Q'}).serialize());
  const edited = save(body); validate(edited); inspect(edited, origins);
  assert.equal(edited.notes['manydoc-note-1'].body.dataStream, 'α\tQFootnote body\r\n');
  fs.writeFileSync(path.join(qa, 'note-origins-edited.json'), JSON.stringify(edited));
  const label = structuredClone(saved);
  label.notes['manydoc-note-1'].body = core.TextX.apply(label.notes['manydoc-note-1'].body, new core.TextX().delete(1).insert(1, {dataStream: 'β', textRuns: [{st: 0, ed: 1, ts: {bl: 1, fs: 13}}]}).serialize());
  const marked = save(label); validate(marked); inspect(marked, origins, 'β');
  assert.equal(marked.notes['manydoc-note-1'].body.dataStream, 'β\tFootnote body\r\n');
  fs.writeFileSync(path.join(qa, 'note-origins-marked.json'), JSON.stringify(marked));
  const detach = structuredClone(saved);
  const identity = records(detach).find(r => r.name === 'manydoc.note-identities.v1');
  const bindings = JSON.parse(identity.data); bindings.bindings = bindings.bindings.filter(b => b.nativeId !== 'manydoc-note-3');
  identity.data = JSON.stringify(bindings);
  const detached = save(detach); validate(detached); inspect(detached, origins);
  assert.equal(detached.body.dataStream, saved.body.dataStream);
  fs.writeFileSync(path.join(qa, 'note-origins-detached.json'), JSON.stringify(detached));
  console.log('Pinned3 note-source/inference records, scoped identities, text-free intent, styled emoji/body mark, two saves/reopens, actual TextX body/label edits and detached identity pass; Go checks historical/unbound semantics. Browser paint remains unqualified.');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
