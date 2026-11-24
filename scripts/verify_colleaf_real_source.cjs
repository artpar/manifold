/* Optional actual exported-PDF source graph qualification; no production Node dependency. */
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const zlib = require('node:zlib');
const { createRequire, stripTypeScriptTypes } = require('node:module');
const root = path.resolve(__dirname, '..');
const qa = path.join(root, 'output/colleaf-qa');
const req = createRequire(path.join(qa, 'package.json'));
const core = req('@univerjs/core');
const docs = req('@univerjs/docs');
const drawing = req('@univerjs/drawing');
const docsDrawing = req('@univerjs/docs-drawing');
const hyperlink = req('@univerjs/docs-hyper-link');
const { FUniver } = req('@univerjs/core/facade');
req('@univerjs/docs/facade');
for (const name of ['core', 'engine-render', 'docs', 'drawing', 'docs-drawing', 'docs-hyper-link']) {
  assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
}
function evidence(data) {
  const carrier = data.body.customRanges.find(r => r.properties?.profile === 'manydoc.native-intent');
  assert.ok(carrier);
  const p = carrier.properties;
  assert.equal(p.version, 2);
  assert.equal(p.records, undefined);
  const raw = zlib.gunzipSync(Buffer.from(p.gzipRecords, 'base64'));
  assert.equal(raw.length, p.recordsBytes);
  return JSON.parse(JSON.parse(raw).find(r => r.name === 'manydoc.document-properties.v1').data).evidence;
}
function save(data) {
  const univer = new core.Univer();
  try {
    for (const plugin of [docs.UniverDocsPlugin, drawing.UniverDrawingPlugin, docsDrawing.UniverDocsDrawingPlugin, hyperlink.UniverDocsHyperLinkPlugin]) univer.registerPlugin(plugin);
    return JSON.parse(JSON.stringify(FUniver.newAPI(univer).createDocument(structuredClone(data)).save()));
  } finally { univer.dispose(); }
}
async function main() {
  const source = fs.readFileSync(path.join(root, 'output/codec/colleaf-format-0.txt'), 'utf8');
  assert.equal(crypto.createHash('sha256').update(source).digest('hex'), 'c54b27b8c303713f5d5d5dfdb7cd54ec9b8410f5045613150861855b11c899c0');
  const app = await import('data:text/javascript;base64,' + Buffer.from(stripTypeScriptTypes(source)).toString('base64'));
  const validate = document => app.validateDocument({ type: 'vibedada-document', version: 3, engine: 'univer', engineVersion: '1.0.4', document });
  const input = JSON.parse(fs.readFileSync(path.join(qa, 'real-source.json')));
  validate(input);
  const original = evidence(input);
  assert.equal(app.documentLimits.bytes, 5 * 1024 * 1024);
  // Reproduce the old transport failure against the same unmodified validator.
  const legacy = structuredClone(input);
  const old = legacy.body.customRanges.find(r => r.properties?.profile === 'manydoc.native-intent').properties;
  old.records = JSON.parse(zlib.gunzipSync(Buffer.from(old.gzipRecords, 'base64')));
  old.version = 1;
  delete old.gzipRecords;
  delete old.recordsBytes;
  assert.throws(() => validate(legacy), /5 MiB/);
  assert.equal(original.pages.length, 10);
  assert.ok(original.origins.length > 100);
  assert.ok(original.runOrigins.length > 100);
  assert.ok(original.listOrigins.length >= 67);
  assert.ok(original.pdfNavigation);
  const saved = save(save(input));
  validate(saved);
  assert.deepEqual(evidence(saved), original);
  assert.equal(saved.body.dataStream, input.body.dataStream);
  assert.deepEqual(saved.body.textRuns, input.body.textRuns);
  assert.deepEqual(saved.body.paragraphs, input.body.paragraphs);
  assert.deepEqual(saved.lists, input.lists);
  fs.writeFileSync(path.join(qa, 'real-source-saved.json'), JSON.stringify(saved));
  console.log(JSON.stringify({ originalPDFPages: original.pages.length, originalTransportBytes: Buffer.byteLength(JSON.stringify(legacy)), compactTransportBytes: Buffer.byteLength(JSON.stringify(input)), savedTransportBytes: Buffer.byteLength(JSON.stringify(saved)), paragraphOrigins: original.origins.length, runOrigins: original.runOrigins.length, listAndItemOrigins: original.listOrigins.length, saves: 2, qualification: 'Pinned actual validator and registered-plugin saves; Go checks complete SF graph. Browser paint/layout is not qualified.' }));
}
main().catch(error => { console.error(error); process.exitCode = 1; });
