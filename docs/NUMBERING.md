# Numbering formats and counter contexts

Ordered lists keep ProseMirror `order` and add optional `attrs.numbering`, mirrored
by public `Block.Numbering *ListNumbering`. Nil retains the existing decimal label
and implicit nesting contract. An explicit value contains zero-based `level`,
`template`, one `formats` entry per level, fallback `ancestors` before the current
level, and optional `legal` / `independent` booleans. This extends the existing
manydoc-prosemirror/1 profile; no runtime dependency was added.

Supported format names follow OOXML: decimal, decimalZero, upperLetter,
lowerLetter, upperRoman and lowerRoman. Alphabetic values repeat the same letter
on successive passes: 26 = Z, 27 = AA, 28 = BB, 53 = AAA. They do not use Excel
column labels. Zero displays 0; decimalZero pads positive single digits. Roman
values use subtractive numerals and repeated M beyond 3999. These cases have
literal Go expectations and a separately authored 45-case OOXML/LibreOffice probe.

[OOXML LevelText](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.leveltext?view=openxml-3.0.1)
uses %1..%9 counter references. Literal text is retained and references to deeper
levels are ignored. [Legal numbering](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.wordprocessing.islegalnumberingstyle?view=openxml-3.0.1)
renders every referenced level as decimal while retaining the underlying formats.
Templates are bounded to 512 UTF-8 bytes, logical levels to nine, and values to
0..2147483647. PDF marker expansion has a 4096-byte admission budget; oversized
symbolic values remain representable in SF and reject PDF output atomically.
Unsupported SF formats reject; unsupported source formats retain decimal-normalized
labels with diagnostics. Existing bullet normalization/diagnostics remain separate.

## Editing and persistence

SF/DOCX/PDF serialization resolves current enclosing ordered-list counters into
matching ancestor levels on owned containers. This updates nested labels when a
parent's start or item order changes. It does not mutate caller data. Fallback
ancestor values cover resumed or orphaned lists whose original ancestors are not
inside the current tree. `independent: true` preserves those values even inside
another list; import uses it when inferred containment joins different Word
numbering identities. Notes and running stories resolve independently. Tables
inside items can retain their surrounding counter context.

Raw DOCX import now retains explicit formats, templates, logical levels and counter
contexts, including standard start/restart overrides, style inheritance and isLgl.
An implicit ancestor encountered through a deeper item initializes its counter;
a following real ancestor advances it. Empty instance level overrides use Word's
zero-start compatibility behavior, with `numbering_empty_override` disclosure.
The original publisher's 1.0 heading proves the required case; this differs from
LibreOffice's behavior on the separately authored implicit-ancestor probe. See
[the LibreOffice developer's change description](https://gerrit.libreoffice.org/c/core/+/145770)
and the linked commit announcement in the QA research logs. This is a bounded
compatibility policy, not a claim to implement every Word counter rule.

DOCX emits ordinary multilevel definitions, starts at both the abstract and
instance override levels, and standard numbered paragraphs. Redundant starts
avoid LibreOffice restarting the footnote example at a despite an instance start
of 28. For legal numbering, the visible format definitions are decimal at every
referenced level; checked ignorable `numberingFormats` metadata retains the
original formats. This avoids LibreOffice ignoring isLgl and painting B.iv.07
instead of 2.4.7. The importer checks those visible definitions before using the
metadata. Implicit/default and independent markers are checked too. Removing
metadata retains the same visible legal labels in a normal OOXML reader.

Numbering font/color/run presentation is diagnosed as normalized. Counter labels
are not part of paragraph run text or plain-text body oracles. No source fixtures,
original hashes, paragraph text hashes or publisher PDF oracles were rewritten.
The existing hierarchy/restart test adds literal input definition/context fields;
its previous hierarchy/start/restart/text expectations remain intact. Its unsupported
format case now uses ordinal because lowerLetter has become supported.

## Independent evidence and remaining work

All ten markers in the all-story example agree between native PDF and an actual
LibreOffice DOCX render, including Appendix AA./BB., AA.iv)/BB.iv), legal 2.4.7,
Roman 4000, table (aaa), footnote (bb), and running H01:/Fix:. Source SF and
DOCX-recovered SF are exactly equal, including stories and notes. Both PDF pages
were visually inspected. The PDF-recovered SF preserves painted marker text and
positions; the skip-inference example does not claim original counter semantics
can be recovered from those pixels/text alone. Actual ProseMirror checks 149
nodes/marks/attributes in the three example trees.

An independent Python formatter compares all 291 numbered source headings with
the unchanged 137-page publisher PDF. 290 complete numbered headings match after
whitespace normalization. The remaining original pair has Word U+2011 in
Non‑shared and PDF ASCII hyphen in Non-shared; that one title spelling difference
is explicitly recorded, and its label 4.8.1 matches. No generic punctuation or
numbering normalization masks mismatches. All 291 exact source-numbered headings
are also present in the native 189-page full-source render. The checker additionally
requires all 291 painted body markers at the native heading size with their title
on the same baseline, so cached TOC text cannot mask wrong body numbering. It retains 434 URI
targets, the original JPEG, required characters and independent font checks.
Native checks additionally require actual Go-family heading markers at the native
heading size and titles alongside on the same baseline, so cached TOC text cannot
mask missing body numbering. All 291 pass. This is heading/marker evidence, not
complete source text/layout qualification.

Remaining work includes internal destinations, broader source/recovery/corpus
qualification, large provenance/codec costs, and explicit recovery of more rich
PDF semantics. No full Word layout or complete numbering-format parity is claimed.
The overall goal remains incomplete. See TESTING.md and verification.json for
actual completed commands/results.

The 45-case probe uses a 22-inch square canvas to keep long repeated-letter labels
visible; expected labels and counter values are unchanged. It independently checks
Go import of every format/template/start and LibreOffice literal labels. The two
implicit-parent sequences are separate research evidence, including the explicitly
documented Word/LibreOffice empty-override disagreement.


## Tagged PDF lists: displayed pattern recovery

The [standard PDF ListNumbering families](https://opensource.adobe.com/dc-acrobat-sdk-docs/pdfstandards/pdfreference1.7old.pdf)
now select Decimal, UpperRoman, LowerRoman, UpperAlpha or LowerAlpha parsing.
Roman tokens must be canonical; the existing SF/OOXML Latin alphabetic scheme
repeats letters after Z. Two-digit decimal padding can be recovered as decimalZero.
Non-Latin alphabets and other alphabetic sequences are not silently translated.

One maximal token position supplies a candidate start and literal prefix/suffix.
Every current label must regenerate exactly through the SF renderer, with 512-byte
template, 4096-byte label and 32-bit counter limits. None/absent attributes retain
conservative decimal/bullet inference, not speculative Roman/Alpha recognition.
Ambiguity, inconsistent formats/sequences and unsupported percent templates retain
literal content with diagnostics; established mixed-punctuation decimal recovery
may still normalize punctuation with its existing diagnostic.

The logical list hierarchy/format attribute is explicit. Start/template and any
padding are display-derived, recorded in evidence and pdf_list_numbering_inferred.
Nested observed prefixes remain fixed literal text; dynamic ancestor bindings
cannot be established from matching strings alone. Annotation/note/bookmark or
picture-bearing labels stay literal rather than being consumed into an automatic
counter. Marker typography is not an editable numbering property and gets
pdf_list_label_presentation when relevant. General literal/custom list hierarchy
and appearance representation remains unfinished; current fallbacks are not goal
completion exemptions.

The writer emits Decimal for decimalZero and for legal-numbered output, matching
its actual visible family rather than the hidden original format. Six generated
cases/18 items now have exact marker/start/template/format, SF, body-style/link,
DOCX and independent PDF/OOXML checks. The unchanged iv./Body source now requires
a typed lowerRoman list/start4/source label and inference disclosure.

```sh
MANYDOC_PDF_LIST_NUMBERING_QA=output/pdf-list-numbering GOWORK=off go test -run TestGeneratedPDFNumberedListSemantics -count=1 .
output/qa-venv/bin/python scripts/verify_pdf_list_numbering.py
```

No authoring counter state, automatic legal/ancestor intent or Word-equivalent
layout is claimed to have been recovered from PDF.


## Fixed ordered labels recovered from PDF

An explicit standard ordered family can display a constant template without a
counter placeholder. If canonical sequence recovery fails, all current labels
must be identical and the existing renderer must reproduce that exact template
for two different counter values. The 512-byte template bound and normal
validation still apply. This includes an empty label; it does not invent painted
content for an empty Lbl. None/unknown/bullet families and annotated/non-text labels
retain their previous outcomes, and variable custom-label semantics remain open.

The family/hierarchy is explicit, but constant-template intent is inferred from
the current display. Hidden start/restart cannot be recovered: use unused start
one with pdf_list_constant_inferred and start_unobserved evidence, never
start_from_label. Once imported, SF/DOCX retain this declared normalized state.
A conflicting nondecimal attribute with displayed 3. can now retain a literal 3.
template rather than becoming a false decimal counter; its original 3. Body
physical oracle remains unchanged. Source fixture bytes are unchanged.

TestGeneratedPDFConstantListFourPaths covers section/word/percent/empty constants,
exact current markers, body styling/URI, source evidence and SF/DOCX round trips.
Direct DOCX retains the authored unused start 42; only the unobservable PDF state
receives the diagnosed default one.
The independent script checks actual L/LI/Lbl, native/Word displayed labels,
OOXML definitions and the explicit start uncertainty. Four native/Word rendered
section/empty pages were visually inspected; no universal layout claim follows.

```sh
MANYDOC_PDF_CONSTANT_LIST_QA=output/pdf-constant-lists GOWORK=off go test -run 'TestPDFConstant|TestGeneratedPDFConstant' -count=1 .
mkdir -p output/pdf-constant-lists/word
/Applications/LibreOffice.app/Contents/MacOS/soffice -env:UserInstallation=file:///tmp/manydoc-constant-list-qa --headless --convert-to pdf --outdir output/pdf-constant-lists/word output/pdf-constant-lists/*.docx
output/qa-venv/bin/python scripts/verify_pdf_constant_lists.py
GOWORK=off go test -run '^$' -fuzz '^FuzzPDFConstantListNumbering$' -fuzztime=10s -parallel=2 .
```
