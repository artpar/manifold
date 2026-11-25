/* Optional actual exported-DOCX and language-style save/edit qualification. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { createRequire, stripTypeScriptTypes } = require('node:module');
const root = path.resolve(__dirname, '../../..');
const qa = path.join(root, 'output/colleaf-qa');
const req = createRequire(path.join(qa, 'package.json'));
const core = req('@univerjs/core'), render = req('@univerjs/engine-render');
const docs = req('@univerjs/docs'), drawing = req('@univerjs/drawing');
const docsDrawing = req('@univerjs/docs-drawing'), hyperlink = req('@univerjs/docs-hyper-link');
const { FUniver } = req('@univerjs/core/facade');
req('@univerjs/docs/facade');
for (const name of ['core', 'engine-render', 'docs', 'drawing', 'docs-drawing', 'docs-hyper-link']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
}
function save(data) {
  const univer = new core.Univer();
  try {
    for (const plugin of [docs.UniverDocsPlugin, drawing.UniverDrawingPlugin, docsDrawing.UniverDocsDrawingPlugin, hyperlink.UniverDocsHyperLinkPlugin]) univer.registerPlugin(plugin);
    return JSON.parse(JSON.stringify(FUniver.newAPI(univer).createDocument(structuredClone(data)).save()));
  } finally { univer.dispose(); }
}
function bodies(data) {
  return [data.body, ...Object.values(data.headers || {}).map(s => s.body), ...Object.values(data.footers || {}).map(s => s.body), ...Object.values(data.notes || {}).map(s => s.body)];
}
function checkStyles(input, output) {
  const before = bodies(input), after = bodies(output);
  assert.equal(before.length, after.length);
  let languageRuns = 0;
  for (let i = 0; i < before.length; i++) {
    assert.equal(after[i].dataStream, before[i].dataStream);
    assert.deepEqual(after[i].textRuns, before[i].textRuns);
    for (const r of after[i].textRuns || []) {
      const marker = r.ts?.manydocLanguages;
      if (!marker) continue;
      languageRuns++;
      assert.equal(marker.profile, 'manydoc.run-languages');
      assert.equal(marker.version, 1);
      assert.equal(marker.alternateKind, 'eastAsia');
      assert.equal(typeof marker.complexLanguage, 'string');
      assert.ok(['en-US', 'ja-JP'].includes(r.ts.altLang));
    }
  }
  assert.ok(languageRuns > 0);
  return languageRuns;
}
function inspectGlyph(data) {
  const model = new core.DocumentDataModel(data);
  const view = new render.DocumentViewModel(model);
  const skeleton = new render.DocumentSkeleton(view, new core.LocaleService());
  try {
    skeleton.calculate();
    let glyph;
    for (const p of skeleton.getSkeletonData().pages) for (const s of p.sections) for (const c of s.columns) for (const l of c.lines) for (const d of l.divides) glyph ||= d.glyphGroup.find(g => g.content === '日');
    assert.ok(glyph);
    assert.equal(glyph.ts.lang, 'en-US');
    assert.equal(glyph.ts.altLang, 'ja-JP');
    assert.equal(glyph.ts.manydocLanguages.complexLanguage, 'ar-SA');
    assert.equal(glyph.ts.bl, 1);
    assert.equal(glyph.ts.fs, 13);
  } finally { skeleton.dispose(); view.dispose(); model.dispose(); }
}
async function main() {
  const source = fs.readFileSync(path.join(root, 'output/codec/colleaf-format-0.txt'), 'utf8');
  assert.equal(crypto.createHash('sha256').update(source).digest('hex'), 'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
  const app = await import('data:text/javascript;base64,' + Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
  const validate = document => app.validateDocument({ type: 'vibedada-document', version: 3, engine: 'univer', engineVersion: '1.0.4', document });
  const input = JSON.parse(fs.readFileSync(path.join(qa, 'languages.json')));
  assert.equal(Object.keys(input).length, 6);
  const saved = {};
  const counts = {};
  for (const [name, data] of Object.entries(input)) {
    validate(data);
    saved[name] = save(save(data));
    validate(saved[name]);
    counts[name] = checkStyles(data, saved[name]);
  }
  inspectGlyph(saved.synthetic);
  fs.writeFileSync(path.join(qa, 'languages-saved.json'), JSON.stringify(saved));
  const edited = structuredClone(saved);
  edited.synthetic.body.textRuns[0].ts.altLang = 'zh-Hant';
  edited.synthetic.body.textRuns[0].ts.manydocLanguages.complexLanguage = 'he-IL';
  edited.synthetic = save(edited.synthetic);
  validate(edited.synthetic);
  fs.writeFileSync(path.join(qa, 'languages-edited.json'), JSON.stringify(edited));
  const cleared = structuredClone(saved);
  delete cleared.synthetic.body.textRuns[0].ts.altLang;
  cleared.synthetic = save(cleared.synthetic);
  validate(cleared.synthetic);
  assert.equal(cleared.synthetic.body.textRuns[0].ts.altLang, undefined);
  fs.writeFileSync(path.join(qa, 'languages-cleared.json'), JSON.stringify(cleared));
  console.log(JSON.stringify({ languageRuns: counts, saves: 2, edits: ['native alternate language', 'separate complex language', 'alternate language removal'], scope: 'Actual validator/plugins/save and glyph-style propagation; browser paint and separate complex-language shaping are not qualified.' }));
}
main().catch(error => { console.error(error); process.exitCode = 1; });
