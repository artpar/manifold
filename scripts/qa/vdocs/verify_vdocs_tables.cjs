/* Optional structural/geometry and mutation QA against pinned public SDK APIs. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { stripTypeScriptTypes } = require('node:module');
const root = path.resolve(__dirname, '../../..');
const qa = path.join(root, 'output/colleaf-qa');
const core = require(path.join(qa, 'node_modules/@univerjs/core'));
const engine = require(path.join(qa, 'node_modules/@univerjs/engine-render'));
for (const name of ['core', 'engine-render']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
}
async function main() {
const snapshot = JSON.parse(fs.readFileSync(path.join(qa, 'tables.json')));
const sourceCode = fs.readFileSync(path.join(root,'output/codec/colleaf-format-0.txt'),'utf8');
assert.equal(crypto.createHash('sha256').update(sourceCode).digest('hex'),'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
const app = await import('data:text/javascript;base64,'+Buffer.from(stripTypeScriptTypes(sourceCode)).toString('base64'));
const envelope = document => ({type:'vibedada-document',version:3,engine:'univer',engineVersion:'1.0.4',document});
app.validateDocument(envelope(snapshot));
const model = new core.DocumentDataModel(snapshot);
let view = new engine.DocumentViewModel(model);
const records = snapshot.body.tables;
const outer = records.find(t => snapshot.tableSource[t.tableId].tableColumns.length === 3);
assert.ok(outer);
const source = snapshot.tableSource[outer.tableId];
assert.deepEqual(source.tableRows.map(r => r.tableCells.map(c => [c.rowSpan,c.columnSpan])), [ [[2,2],[1,1]], [[0,2],[1,1]], [[1,3]] ]);
assert.deepEqual(source.tableColumns.map(c => c.size.width), [{v:96,u:3},{v:144,u:3},{v:192,u:3}]);
const parsed = engine.parseDataStreamToTree(snapshot.body.dataStream, snapshot.body.tables);
for (const record of records) {
  const table = view.getTableByStartIndex(record.startIndex);
  assert.equal(table.table.endIndex,record.endIndex);
  const tree = parsed.tableNodeCache.get(record.tableId).table;
  assert.equal(tree.children.length, table.tableSource.tableRows.length);
  tree.children.forEach((row, i) => assert.equal(row.children.length,table.tableSource.tableRows[i].tableCells.length));
}
// Legacy unit0 is labelled POINT in the declarations, but table layout consumes
// it as geometry pixels, just like unit3. Do not apply a second 4/3 conversion.
const legacy = structuredClone(snapshot);
for (const t of Object.values(legacy.tableSource)) {
  if (t.size.width.u===3) {t.size.width.u=0;for(const col of t.tableColumns)col.size.width.u=0;}
}
const legacyModel = new core.DocumentDataModel(legacy);
const legacyView = new engine.DocumentViewModel(legacyModel);
const legacySkeleton = new engine.DocumentSkeleton(legacyView,new core.LocaleService());
legacySkeleton.calculate();
assert.equal([...legacySkeleton.getSkeletonData().pages[0].skeTables.values()].find(t=>t.tableId===outer.tableId).width,432);
legacySkeleton.dispose();legacyView.dispose();legacyModel.dispose();
let skeleton = new engine.DocumentSkeleton(view,new core.LocaleService());
skeleton.calculate();
// skeTables is a Map, so ordinary JSON stringification conceals its entries.
const tables = skeleton.getSkeletonData().pages.flatMap(p => [...p.skeTables.values()]);
const actual = tables.find(t => t.tableId===outer.tableId);
assert.ok(actual);
assert.equal(actual.width,432);
assert.equal(actual.rows[0].cells[0].left,0);
assert.equal(actual.rows[0].cells[1].left,240);
assert.equal(actual.rows[1].cells[1].left,240); // vertical continuation must advance two columns
assert.equal(actual.rows[2].cells[0].left,0);
assert.equal(actual.rows[1].cells[0].st,0); // native suppressed continuation skeleton
const innerPages = actual.rows[0].cells[0];
assert.ok(innerPages.skeTables instanceof Map);
assert.equal(innerPages.skeTables.size,1);
const autoRecord = records.find(t => snapshot.tableSource[t.tableId].tableRows[0].repeatHeaderRow === 1);
assert.ok(autoRecord);
const autoTable = tables.find(t => t.tableId===autoRecord.tableId);
assert.ok(autoTable.width > 0, 'native auto-width table must resolve to visible geometry');
const changeOffset = parsed.tableNodeCache.get(outer.tableId).table.children[1].children[1].startIndex+1;
assert.equal(snapshot.body.dataStream.slice(changeOffset,changeOffset+1),'C');
skeleton.dispose();view.dispose();
const actions = new core.TextX().retain(changeOffset).delete(1).insert(1,{dataStream:'Z'}).serialize();
const body = core.TextX.apply(structuredClone(model.getBody()),actions);
model.reset({...model.getSnapshot(),body});
view = new engine.DocumentViewModel(model);
assert.equal(model.getBody().dataStream.slice(changeOffset,changeOffset+1),'Z');
assert.deepEqual(model.getSnapshot().tableSource[outer.tableId],source);
assert.equal(view.getTableByStartIndex(outer.startIndex).tableSource.tableRows[1].tableCells[0].rowSpan,0);
app.validateDocument(envelope(model.getSnapshot()));
fs.writeFileSync(path.join(qa,'tables-edited.json'),JSON.stringify(model.getSnapshot(),null,2));
view.dispose();model.dispose();
console.log('Pinned native nested/merged/auto-width table records, cell geometry and TextX edit/save passed; font metrics/browser rendering not qualified');

}
main().catch(error=>{console.error(error);process.exitCode=1;});
