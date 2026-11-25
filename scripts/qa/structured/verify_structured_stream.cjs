// Optional ProseMirror consumer of JSONL body nodes; bounded memory for huge SF.
// Feed complete nodes from the independent streaming JSON parser in huge_audit.py.
const fs = require('node:fs');
const path = require('node:path');
const readline = require('node:readline');
const {Schema} = require('prosemirror-model');
const schema = new Schema(JSON.parse(fs.readFileSync(path.join(__dirname, '../../../schema/prosemirror-v1.json'))));
let count=0;
readline.createInterface({input:process.stdin, crlfDelay:Infinity}).on('line', line => {
 const n=JSON.parse(line); schema.nodeFromJSON(n).check(); count++;
}).on('close',()=>{if(!count)process.exitCode=1;console.log(JSON.stringify({prosemirror_body_nodes_checked:count}));});
