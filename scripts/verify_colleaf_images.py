"""Optional independent Pillow inspection of embedded native assets; QA only."""
import base64
import hashlib
import io
import json
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parent.parent
snapshot = json.loads((root / "output/colleaf-qa/images.json").read_text())
images = list(snapshot["drawings"].values())
for note in snapshot["notes"].values():
    images.extend(note.get("drawings", {}).values())
assert len(images) == 5
counts = {"PNG": 0, "JPEG": 0}
for drawing in images:
    raw = base64.b64decode(drawing["source"].split(",", 1)[1], validate=True)
    image = Image.open(io.BytesIO(raw))
    image.load()
    counts[image.format] += 1
    if image.format == "PNG":
        assert image.size == (32, 16)
        rgba = image.convert("RGBA")
        assert rgba.getpixel((0, 0)) == (200, 30, 50, 255)
        assert rgba.getpixel((31, 15)) == (20, 80, 220, 128)
    else:
        assert hashlib.sha256(raw).hexdigest() == "fe9737d0cb4f905917054c7938fc3ccf4b83f0623df9cd5d62192dbaae26b510"
assert counts == {"PNG": 3, "JPEG": 2}
print("Independent Pillow: three alpha PNGs and two original JPEG occurrences decode with expected pixels/bytes")
