/* Optional pinned native SDK facade save qualification. */
const assert = require('node:assert/strict'), fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto');
const {createRequire} = require('node:module');
const root = path.resolve(__dirname, '../../..'), qa = path.join(root, 'output/vdocs-qa'), req = createRequire(path.join(qa, 'package.json'));
const core = req('@univerjs/core'), docs = req('@univerjs/docs'), drawing = req('@univerjs/drawing'), dd = req('@univerjs/docs-drawing');
const {FUniver} = req('@univerjs/core/facade'); req('@univerjs/docs/facade');
for (const name of ['core', 'docs', 'drawing', 'docs-drawing']) assert.equal(JSON.parse(fs.readFileSync(path.join(qa, 'node_modules/@univerjs', name, 'package.json'))).version, '1.0.4');
function save(document) {
 const u = new core.Univer();
 try {
  for (const plugin of [docs.UniverDocsPlugin, drawing.UniverDrawingPlugin, dd.UniverDocsDrawingPlugin]) u.registerPlugin(plugin);
  return JSON.parse(JSON.stringify(FUniver.newAPI(u).createDocument(structuredClone(document)).save()));
 } finally { u.dispose(); }
}
async function main() {

 const wire = JSON.parse(fs.readFileSync(process.argv[2] || path.join(qa, 'sections.vdocs')));

 assert.equal(wire.type, 'manifold-document'); assert.equal(wire.version, 3); assert.equal(wire.engine, 'univer'); assert.equal(wire.engineVersion, '1.0.4');
 assert.equal(wire.document.locale, 'enUS');
 const saved = {...wire, document: save(save(wire.document))};
 assert.equal(saved.document.body.dataStream, wire.document.body.dataStream);
 assert.deepEqual(saved.document.body.sectionBreaks, wire.document.body.sectionBreaks);
 // SDK initializes absent story/drawing maps to empty objects. No entries
 // may appear or disappear; only that schema-equivalent empty default differs.
 assert.deepEqual(saved.document.headers ?? {}, wire.document.headers ?? {});
 assert.deepEqual(saved.document.footers ?? {}, wire.document.footers ?? {});
 assert.deepEqual(saved.document.drawings ?? {}, wire.document.drawings ?? {});
 fs.writeFileSync(process.argv[3] || path.join(qa, 'sections-saved.vdocs'), JSON.stringify(saved));
 // Snapshot save checks establish transport acceptance only; the other
 // section/glyph probes and future browser qualification retain their own scope.
 console.log('Manifold .vdocs envelope and pinned native main/section/running/drawing facade save/reopen transport checked; Go compares full SF graph separately');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
