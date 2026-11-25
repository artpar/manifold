"""Optional independent pypdf/MuPDF audit of actual painted output structure.
Run with output/qa-venv/bin/python scripts/qa/layout/verify_generated_pdf_structure.py
No production Python dependency or cached SF/text oracle is used.
"""
import argparse
import collections
import hashlib
import json
import re
from pathlib import Path

from pypdf import PdfReader
from pypdf.generic import ContentStream, IndirectObject

parser = argparse.ArgumentParser()
parser.add_argument("pdf", nargs="?", default="output/colleaf-tagged/document.pdf")
parser.add_argument("--baseline", default="output/colleaf-baseline-1d13/document.pdf")
parser.add_argument("--source-sf", default="examples/notes/document.sf")
args = parser.parse_args()
source = json.loads(Path(args.source_sf).read_text())
pdf = PdfReader(args.pdf, strict=True)
assert pdf.pdf_header == "%PDF-2.0"
root = pdf.trailer["/Root"]
tree = root["/StructTreeRoot"]
assert root["/MarkInfo"]["/Marked"]
assert tree["/Namespaces"][0].get_object()["/NS"] == "http://iso.org/pdf2/ssn"


def key(ref):
    assert isinstance(ref, IndirectObject), ref
    return ref.idnum, ref.generation


def array(value):
    if isinstance(value, IndirectObject):
        resolved = value.get_object()
        return resolved if isinstance(resolved, list) else [value]
    return value if isinstance(value, list) else [value]


page_by_ref = {key(p.indirect_reference): i for i, p in enumerate(pdf.pages)}
roles = collections.Counter()
nodes = {}
mcrs = {}
notes = []
references = []


def walk(ref, parent):
    ident = key(ref)
    assert ident not in nodes, ("cyclic or multiply owned structure", ident)
    node = ref.get_object()
    assert node["/Type"] == "/StructElem"
    assert key(node.raw_get("/P")) == key(parent)
    assert node["/NS"]["/NS"] == "http://iso.org/pdf2/ssn"
    nodes[ident] = node
    role = node["/S"]
    roles[role] += 1
    if role == "/FENote":
        notes.append((ref, node))
    if role == "/Reference":
        references.append((ref, node))
    for child in array(node.raw_get("/K")):
        value = child.get_object()
        if value.get("/Type") == "/MCR":
            location = (page_by_ref[key(value.raw_get("/Pg"))], int(value["/MCID"]))
            assert location not in mcrs
            mcrs[location] = ident
        else:
            walk(child, ref)


for ref in array(tree.raw_get("/K")):
    walk(ref, root.raw_get("/StructTreeRoot"))
nums = tree["/ParentTree"]["/Nums"]
parents = {int(nums[i]): nums[i + 1] for i in range(0, len(nums), 2)}
assert len(parents) == len(pdf.pages)
painted = {}
for i, page in enumerate(pdf.pages):
    owners = parents[int(page["/StructParents"])]
    stack = []
    mcids = []
    paint_counts = collections.Counter()
    for operands, operator in ContentStream(page["/Contents"], pdf).operations:
        if operator in (b"BDC", b"BMC"):
            mcid = None
            if operator == b"BDC" and "/MCID" in operands[1]:
                mcid = int(operands[1]["/MCID"])
                mcids.append(mcid)
                assert key(owners[mcid]) == mcrs[(i, mcid)]
                painted[(i, mcid)] = key(owners[mcid])
            stack.append(mcid)
        elif operator == b"EMC":
            assert stack, "unbalanced marked content"
            stack.pop()
        elif operator in (b"Tj", b"TJ", b"Do"):
            for mcid in reversed(stack):
                if mcid is not None:
                    paint_counts[mcid] += 1
                    break
    assert not stack, "unclosed marked content"
    assert mcids == list(range(len(owners))), (i, mcids)
    assert all(paint_counts[m] for m in mcids), "empty owner masquerading as painted content"
assert painted == mcrs
def sf_nodes(items):
    for node in items:
        yield node
        yield from sf_nodes(node.get("content", []))

source_notes = source.get("attrs", {}).get("notes", [])
logical_nodes = list(sf_nodes(source.get("content", [])))
for note in source_notes:
    logical_nodes += list(sf_nodes(note.get("content", [])))
expected_refs = sum(n["type"] == "note_reference" for n in logical_nodes)
assert len(notes) == len(source_notes) and len(references) == expected_refs
kinds = collections.Counter(n["/A"]["/NoteType"] for _, n in notes)
assert kinds == collections.Counter("/" + n["kind"].title() for n in source_notes)
for sf_role, pdf_role in [("table", "/Table"), ("table_row", "/TR"), ("image", "/Figure")]:
    assert roles[pdf_role] == sum(n["type"] == sf_role for n in logical_nodes)
source_spans = collections.Counter((n.get("attrs", {}).get("rowspan", 1), n.get("attrs", {}).get("colspan", 1)) for n in logical_nodes if n["type"] == "table_cell")
output_spans = collections.Counter((int(n.get("/A", {}).get("/RowSpan", 1)), int(n.get("/A", {}).get("/ColSpan", 1))) for n in nodes.values() if n["/S"] in ("/TD", "/TH"))
assert source_spans == output_spans
for ref, node in references:
    targets = array(node["/Ref"])
    assert len(targets) == 1
    target = targets[0].get_object()
    assert target["/S"] == "/FENote"
    assert key(ref) in {key(r) for r in array(target["/Ref"])}
ids = tree["/IDTree"]["/Names"]
assert len(ids) == 2 * len(notes)
assert all(ids[i] == ids[i + 1].get_object()["/ID"] for i in range(0, len(ids), 2))
attachments = root.get("/Names", {}).get("/EmbeddedFiles", {})
assert not attachments.get("/Names") and not attachments.get("/Kids"), "embedded document cache"
text = "\n".join(p.extract_text() for p in pdf.pages)
expected_tokens = collections.Counter()
repeatable = set()
def source_tokens(items, repeated=False):
    for node in items:
        current = repeated or (node["type"] == "table_row" and node.get("attrs", {}).get("repeatHeader", False))
        if node["type"] == "text":
            tokens = re.findall(r"\b[A-Z][A-Z_]{3,}(?:_\d+)?\b", node["text"])
            expected_tokens.update(tokens)
            if current:
                repeatable.update(tokens)
        source_tokens(node.get("content", []), current)
source_tokens(source.get("content", []))
for note in source_notes:
    source_tokens(note.get("content", []))
# Superscript citation digits can directly follow a source token in pypdf's
# plain text. Match the known literal token, rather than invent a word boundary
# between that token and its separately verified Reference owner.
for token, expected in expected_tokens.items():
    actual = text.count(token)
    assert actual >= expected if token in repeatable else actual == expected, (token, expected, actual)
result = {"pages": len(pdf.pages), "roles": dict(roles), "painted_MCID_owners": len(painted), "note_kinds": dict(kinds), "citations": len(references), "source_marker_tokens": len(expected_tokens), "scope": "Painted role/parent/MCID/reciprocal-Ref/ID graph; not PDF/UA or WTPDF conformance"}
if args.baseline:
    import pymupdf as fitz
    baseline = fitz.open(args.baseline)
    current = fitz.open(args.pdf)
    assert len(baseline) == len(current)
    digests = []
    for a, b in zip(baseline, current):
        old = a.get_pixmap(matrix=fitz.Matrix(1.5, 1.5), alpha=False)
        new = b.get_pixmap(matrix=fitz.Matrix(1.5, 1.5), alpha=False)
        assert old.width == new.width and old.height == new.height
        assert old.samples == new.samples, "tagging changed rendered pixels"
        digests.append(hashlib.sha256(new.samples).hexdigest())
    result["MuPDF_baseline_pixels"] = {"scale": 1.5, "equal_pages": len(digests), "sha256": digests}
print(json.dumps(result, indent=2))
