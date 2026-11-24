"""Independent ParentTree audit/mutation using optional pypdf (QA only).

Run with output/qa-venv/bin/python scripts/verify_pdf_parent_tree.py INPUT.pdf
Writes a contradictory-owner variant beside OUTPUT_PREFIX; never changes source.
"""
import argparse
from pathlib import Path
from pypdf import PdfReader, PdfWriter
from pypdf.generic import ArrayObject, DictionaryObject, IndirectObject

parser = argparse.ArgumentParser()
parser.add_argument("pdf")
parser.add_argument("--out", default="output/pdf-parent-tree/mutated.pdf")
args = parser.parse_args()
reader = PdfReader(args.pdf, strict=True)
tree = reader.trailer["/Root"]["/StructTreeRoot"]

def ident(ref):
    assert isinstance(ref, IndirectObject), ("owner is not indirect", ref)
    return ref.idnum, ref.generation

def numbers(node):
    node = node.get_object()
    if "/Kids" in node:
        result = {}
        for child in node["/Kids"]:
            for key, val in numbers(child).items():
                assert key not in result
                result[key] = val
        return result
    nums = node["/Nums"]
    assert len(nums) % 2 == 0
    keys = [int(nums[i]) for i in range(0, len(nums), 2)]
    assert keys == sorted(set(keys))
    return {key: nums[2 * i + 1] for i, key in enumerate(keys)}

index = numbers(tree["/ParentTree"])
expected = {}
owners = {}
containers = {}

def content(item, page, owner):
    obj = item.get_object()
    if isinstance(obj, int):
        mcid, container = obj, page
    elif obj.get("/Type") == "/OBJR":
        mcid, container = -1, obj["/Obj"]
    elif obj.get("/Type") == "/MCR":
        mcid, container = int(obj["/MCID"]), obj.get("/Stm", page)
    else:
        walk(item, page)
        return
    container = container.get_object()
    key = int(container["/StructParent" if mcid < 0 else "/StructParents"])
    slot = key, mcid
    assert slot not in expected
    expected[slot] = ident(owner)
    containers[slot] = container

def walk(ref, inherited_page=None):
    owner_id = ident(ref)
    assert owner_id not in owners
    node = ref.get_object()
    owners[owner_id] = ref
    page = node.get("/Pg", inherited_page)
    kids = node.get("/K", [])
    if isinstance(kids, IndirectObject) and isinstance(kids.get_object(), ArrayObject):
        kids = kids.get_object()
    if not isinstance(kids, list):
        kids = [kids]
    for kid in kids:
        val = kid.get_object()
        if isinstance(val, DictionaryObject):
            page_for_item = val.get("/Pg", page)
        else:
            page_for_item = page
        content(kid, page_for_item, ref)

kids = tree.raw_get("/K")
if isinstance(kids, IndirectObject) and isinstance(kids.get_object(), ArrayObject):
    kids = kids.get_object()
if not isinstance(kids, list):
    kids = [kids]
for kid in kids:
    walk(kid)
for (key, mcid), owner in expected.items():
    entry = index[key]
    actual = entry.get_object()[mcid] if mcid >= 0 else entry
    assert ident(actual) == owner, (key, mcid, ident(actual), owner)
print(f"{args.pdf}: {len(owners)} forward elements / {len(expected)} reciprocal content owners pass")

# Change an actual existing slot to another existing forward element, leaving
# every source content stream and forward K entry intact. The importer must
# reject the contradiction rather than silently choose one direction.
slot = next(pos for pos in expected if pos[1] >= 0)
replacement = next(ref for owner_id, ref in owners.items() if owner_id != expected[slot])
key, mcid = slot
index[key].get_object()[mcid] = replacement
writer = PdfWriter(clone_from=reader)
out = Path(args.out)
out.parent.mkdir(parents=True, exist_ok=True)
with out.open("wb") as handle:
    writer.write(handle)
mutated = PdfReader(out, strict=True)
print(f"Wrote contradictory reciprocal-owner mutation: {out}")

assert len(mutated.pages) == len(reader.pages)
for original_page, mutated_page in zip(reader.pages, mutated.pages):
    assert original_page.get_contents().get_data() == mutated_page.get_contents().get_data()
print("Mutation preserves every decoded page content stream")
