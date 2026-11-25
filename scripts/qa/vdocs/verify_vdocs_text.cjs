/* Optional pinned native-reader/edit/save QA. No production JS dependency. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const qa = path.join(root, 'output/vdocs-qa');
const core = require(path.join(qa, 'node_modules/@univerjs/core'));
const engine = require(path.join(qa, 'node_modules/@univerjs/engine-render'));
for (const name of ['core', 'engine-render']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
}
const snapshot = JSON.parse(fs.readFileSync(path.join(qa, 'text.json')));
const model = new core.DocumentDataModel(snapshot);
let view = new engine.DocumentViewModel(model);
assert.equal(model.getBody().dataStream, 'Title 😀\rLink 😀\vnext\tcell\r\r\n');
assert.equal(view.getTextRun(6).ts.bl, 1);
assert.equal(view.getTextRun(6).ts.lang, 'en-US');
assert.equal(view.getTextRun(6).ts.fs, 16); // points, not geometry pixels
assert.equal(view.getCustomRange(15).properties.url, 'https://example.org');
assert.equal(view.getCustomRange(16).properties.bookmarkId, 'destination'); // adjacent range begins after inclusive URL end
assert.equal(model.getBody().paragraphs[0].paragraphStyle.outlineLevel, 5);
assert.equal(model.getBody().paragraphs[0].paragraphStyle.direction, 2);
view.dispose();
// Use the published text mutation API to replace the astral heading text.
const actions = new core.TextX().delete(8).insert(8, {
  dataStream: 'Edited😀', textRuns: [{st:0, ed:8, ts:{it:1}}],
}).serialize();
const edited = core.TextX.apply(structuredClone(model.getBody()), actions);
model.reset({...model.getSnapshot(), body:edited});
const saved = JSON.parse(JSON.stringify(model.getSnapshot()));
view = new engine.DocumentViewModel(model);
assert.equal(saved.body.dataStream.slice(0, 8), 'Edited😀');
assert.equal(view.getTextRun(6).ts.it, 1);
assert.equal(view.getTextRun(6).ts.bl, undefined);
assert.equal(view.getCustomRange(15).properties.url, 'https://example.org');
fs.writeFileSync(path.join(qa, 'text-edited.json'), JSON.stringify(saved, null, 2));
view.dispose(); model.dispose();
console.log('Pinned native text/style/link reader and TextX edit/save passed; Go must re-import text-edited.json; no layout qualification');
