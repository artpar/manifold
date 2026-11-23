# Paragraph embedding levels for layout

This is an internal, pure-Go paragraph resolver and line reorderer, qualified
against Unicode 15.0.0. It is connected through pdf_bidi.go to original scalar
ranges, directional shaping and final-line placement. The renderer separately
qualifies supplied-font Hebrew/Arabic glyphs/marks/mirrors, logical text and links;
algorithm conformance is not evidence of complete script or Word support.

`Resolve(ctx, text, base)` accepts exactly one paragraph and an automatic (-1),
explicit LTR (0) or explicit RTL (1) base. It retains original Unicode scalar
indices without normalization. `ResolveWithTypes` accepts validated higher-level-protocol class overrides (used
for run rtl weak/neutral characteristics). `Levels` supplies levels for shaping;
`Line(start,end)` applies L1/L2 from those resolved levels, without re-running P2,
weak/neutral resolution or isolates on the sliced line. Results are owned copies,
allowing concurrent line queries. X9 controls remain in the returned map, with
non-normative placement; a renderer must retain logical text without painting
those controls. L3 mark/grapheme handling and L4 mirroring belong to the shaper.

A 65,536-scalar resource limit bounds paragraph work; malformed UTF-8, invalid
bases/line ranges and interior B-class paragraph boundaries reject. Cancellation
is checked before, during preprocessing and after resolution. Core resolution is
bounded synchronous Go work between those checks, not fully interruptible at
every rule. No runtime service, CGO, font lookup or external executable is used.

## Reused source and changes

`core.go` and `bracket.go` adapt the Go Authors' BSD-licensed
[golang.org/x/text v0.30.0 Unicode bidi reference core](https://github.com/golang/text/tree/v0.30.0/unicode/bidi).
Their original headers and the adjacent LICENSE are retained. This is attributed
open-source reuse, not independently authored algorithm code. Upstream file hashes
are in testdata/manifest.json. The public upstream API hides embedding levels,
returns logical direction runs without their full L2 levels, and re-resolves
sliced input in Line. The internal adapter exposes the stages needed by layout.

The source changes are the package name and a linear stack-based implementation
of matching isolates. Upstream's repeated suffix scan is quadratic on unmatched
initiators. Class values/Unicode property lookup reuse the existing pinned x/text
module; no duplicate character-class table or new Go module is introduced.
The wrapper supplies the published BidiBrackets table and canonical opening
values for U+2329/U+232A versus U+3008/U+3009, rather than raw closing-rune values.

Unicode data and tests retain their notices and the existing
[Unicode License V3](../../third_party/licenses/Unicode-V3.txt). Runtime bracket
data is embedded from the unmodified published file. The two original full test
files are gzip-compressed without changing their bytes; tests check their original
SHA256/size and exact case counts before interpreting them. They are reference
fixtures, not generated application output. No network is needed for tests.

## Verification and limits of the evidence

```sh
GOWORK=off go test ./internal/bidilevels -v
GOWORK=off go test -race ./internal/bidilevels
GOWORK=off go test ./internal/bidilevels -run '^$' -fuzz '^FuzzParagraphLines$' -fuzztime=20000x -parallel=2
python3 scripts/verify_bidi_levels.py
# Optional explicit ICU selection:
MANYDOC_QA_ICU_LIBRARY=/opt/homebrew/opt/icu4c/lib/libicuuc.dylib python3 scripts/verify_bidi_levels.py
```

The ordinary suite includes all 91,707 original BidiCharacterTest cases and 770,241
BidiTest class/direction combinations, with original x exclusions for X9 controls.
These cover UAX #9 levels/order, not shaping, font coverage or glyph rendering.
Line tests preserve independent native FriBidi 1.0.16 and ICU 78.3 observations for
12 authored paragraphs/34 wrapped lines, including numbers, nested brackets,
canonical bracket equivalence, explicit bases, isolates, embeddings and marks.
The optional read-only Python verifier uses locally installed native libraries;
none is part of the Go dependency graph. Different native versions may legitimately
need a new, separately attributed observation, never an automatic oracle rewrite.

Native observations disagree on one trailing PDI: FriBidi keeps level 1 at the
line end; UAX #9 L1 resets it to paragraph level 0 and ICU agrees. Both observations
remain in lines.json; tests require the Unicode/ICU result and preserve the exact
FriBidi difference. ICU also lowers even levels on two unidirectional line slices;
tests compare its direction parity and visual order, consistent with UAX #9
conformance, while the original Unicode corpus checks exact specified levels.

The PDF implementation now carries paragraph scalar ranges/levels through
normalization, grapheme segmentation, current run/script/font segmentation and wrapping,
shapes final lines in their resolved direction, applies L2 to placement units, and
preserves logical extraction independently of paint order. Existing LTR font/GID/
GPOS, paint/ligature and URI/note oracles must stay intact. Broader PDF bidi/joining/itemization, within-grapheme styles and context across
fonts/sizes/languages still need broader qualification. Full HarfBuzz item context
now crosses these boundaries; see the root PDF architecture and context oracles.
