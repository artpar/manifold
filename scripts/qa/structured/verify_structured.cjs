// Optional independent reader; install prosemirror-model in an ignored directory.
// NODE_PATH=output/sf-qa/node_modules node scripts/qa/structured/verify_structured.cjs output/report.sf
const fs = require('node:fs');
const zlib = require('node:zlib');
const path = require('node:path');
const assert = require('node:assert/strict');
const {Schema} = require('prosemirror-model');
const spec = JSON.parse(fs.readFileSync(path.join(__dirname, '../../../schema/prosemirror-v1.json')));
const schema = new Schema(spec);
let count = 0;
function verifyTree(nodes) {
  for (const n of nodes || []) {
    const parsed = schema.nodeFromJSON(n);
    parsed.check();
    assert.equal(parsed.type.name, n.type);
    if (n.text !== undefined) assert.equal(parsed.text, n.text);
    for (const [k, v] of Object.entries(n.attrs || {})) assert.deepEqual(parsed.attrs[k], v);
    for (const mark of n.marks || []) {
      const actual = parsed.marks.find(m => m.type.name === mark.type);
      assert.ok(actual, `missing mark ${mark.type}`);
      for (const [k, v] of Object.entries(mark.attrs || {})) assert.deepEqual(actual.attrs[k], v);
    }
    count++;
    verifyTree(n.content);
    if (n.type === 'note_reference' && n.attrs?.label_content != null) {
      assert.ok(Array.isArray(n.attrs.label_content) && n.attrs.label_content.length);
      assert.ok(n.attrs.label_content.every(n => n.type === 'text'));
      verifyTree(n.attrs.label_content);
    }
    if (n.type === 'section') {
      for (const role of ['headers', 'footers']) {
        for (const [variant, content] of Object.entries(n.attrs?.[role] || {})) {
          assert.ok(['default', 'first', 'even'].includes(variant));
          assert.ok(Array.isArray(content));
          verifyTree(content);
        }
      }
    }
  }
}
for (const filename of process.argv.slice(2)) {
  const bytes = fs.readFileSync(filename);
  const input = JSON.parse(filename.toLowerCase().endsWith('.sf.gz') ? zlib.gunzipSync(bytes).toString('utf8') : bytes.toString('utf8'));
  assert.equal(input.attrs.schema, 'manydoc-prosemirror/1');
  verifyTree([input]);
  verifyTree(input.attrs.headers);
  verifyTree(input.attrs.footers);
  for (const note of input.attrs.notes || []) {
    verifyTree(note.content);
    if (note.body_label_content !== undefined) {
      assert.ok(Array.isArray(note.body_label_content) && note.body_label_content.length);
      assert.ok(note.body_label_content.every(n => n.type === 'text'));
      verifyTree(note.body_label_content);
    }
    if (note.body_label_marks !== undefined) {
      assert.ok(Array.isArray(note.body_label_marks));
      verifyTree([{type: 'text', text: 'label', marks: note.body_label_marks}]);
    }
  }
  console.log(`ProseMirror checked ${filename}`);
}
assert.ok(count, 'provide at least one .sf file');
console.log(`Checked ${count} nodes and their marks/attributes`);
