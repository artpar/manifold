/* Optional independent pinned VDOCS/Univer reader QA, not a runtime dependency. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const crypto = require('node:crypto');
const { stripTypeScriptTypes } = require('node:module');
const path = require('node:path');
const root = path.resolve(__dirname, '../../..');
const qa = path.join(root, 'output/vdocs-qa/node_modules');
const core = require(path.join(qa, '@univerjs/core'));
const engine = require(path.join(qa, '@univerjs/engine-render'));
for (const name of ['core', 'engine-render']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, '@univerjs', name, 'package.json'))).version, '1.0.4');
}
const input = JSON.parse(fs.readFileSync(path.join(root, 'testdata/univer/native-stream.json')));

async function main() {

  assert.equal(core.DataStreamTreeTokenType.TABLE_ROW_END, '\x0e');
  assert.equal(core.DataStreamTreeTokenType.TABLE_END, '\x0f');
  assert.equal(engine.ptToPixel(72), 96);
  assert.equal(engine.pixelToPt(96), 72);
  const names = {SECTION_BREAK:'section', PARAGRAPH:'paragraph', TABLE:'table', TABLE_ROW:'row', TABLE_CELL:'cell'};
  const normalize = (nodes) => nodes.map(n => ({kind:names[n.nodeType],start:n.startIndex,end:n.endIndex+1,children:normalize(n.children)}));
  const actual = engine.parseDataStreamToTree(input.document.body.dataStream, input.document.body.tables);
  const expected = JSON.parse(fs.readFileSync(path.join(root, 'output/vdocs-qa/go-tree.json')));
  assert.deepEqual(normalize(actual.sectionList), expected);
  const model = new core.DocumentDataModel(input.document);
  const view = new engine.DocumentViewModel(model);
  assert.equal(model.getBody().dataStream, input.document.body.dataStream);
  assert.deepEqual(view.getTextRun(1).ts, {bl:1,fs:12,ff:'Go'});
  assert.equal(view.getCustomRange(1).endIndex, 2); // inclusive UTF-16, not Go bytes
  assert.equal(view.getCustomRange(1).properties.url, 'https://example.org');
  assert.equal(view.getCustomBlock(10).noteId, 'n');
  assert.equal(view.getNoteTreeMap().get('n').getBody().dataStream, 'Body 😀\r\n');
  for (const range of input.document.body.tables) {
    const found = view.getTableByStartIndex(range.startIndex);
    assert.equal(found.table.tableId, range.tableId);
    assert.equal(found.table.endIndex, range.endIndex); // exclusive
    assert.equal(found.tableSource.tableRows[0].tableCells.length, range.tableId==='outer'?2:1);
  }
  view.dispose();model.dispose();
  // Native snapshots can carry overlapping/unsorted intervals. The actual
  // reader chooses the last matching whole record, rather than unioning marks.
  const overlap = new core.DocumentDataModel({id:'overlap',documentStyle:{documentFlavor:1},body:{dataStream:'ABCD\r\n',textRuns:[{st:1,ed:4,ts:{it:1}},{st:0,ed:3,ts:{bl:1}},{st:4,ed:4,ts:{bl:1}}],paragraphs:[{startIndex:4}],sectionBreaks:[{startIndex:5}]}});
  const overlapView = new engine.DocumentViewModel(overlap);
  assert.deepEqual(overlapView.getTextRun(2).ts,{bl:1});
  assert.deepEqual(overlapView.getTextRun(3).ts,{it:1});
  assert.equal(overlapView.getTextRun(4),undefined);
  overlapView.dispose();overlap.dispose();
  console.log('Actual pinned Univer UTF-16 style/link/note metadata and nested table/section AST bounds match Go; no bridge/renderer-completion claim');
}
main().catch(error => {console.error(error); process.exitCode=1;});
