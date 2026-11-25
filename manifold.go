// Package manifold reads, writes and converts rich documents in pure Go.
package manifold

import (
	"context"
	"io"

	"github.com/artpar/manifold/internal/engine"
)

const AnchorPoint = engine.AnchorPoint

type Block = engine.Block

type Cell = engine.Cell

const DOCX = engine.DOCX

type Diagnostic = engine.Diagnostic

// Document is manifold's interchange model. Diagnostics describe input features
// whose structure cannot be represented. Converters reject lossy output unless
// the caller explicitly acknowledges those diagnostics.
type Document = engine.Document

const Endnote = engine.Endnote

var ErrLossy = engine.ErrLossy
var ErrUnsupported = engine.ErrUnsupported

// FontFace supplies one embeddable TTF program under a lookup family name.
// Bold/Italic select its request slot; the supplied outlines determine appearance.
// The renderer does not synthesize missing variants.
type FontFace = engine.FontFace

const Footnote = engine.Footnote

type Format = engine.Format

// FormatFromName identifies a supported format from a filename.
func FormatFromName(name string) (Format, error) { return engine.FormatFromName(name) }

// FromStructured validates and converts a ProseMirror tree to the document model.
func FromStructured(s *StructuredDocument) (*Document, error) { return engine.FromStructured(s) }

const Heading = engine.Heading

type Image = engine.Image

// OfficeMath retains native OOXML equations through SF and DOCX.
type OfficeMath = engine.OfficeMath

const OfficeMathBlock = engine.OfficeMathBlock

const JSON = engine.JSON

type Kind = engine.Kind

// Layout dimensions are points, 72 per inch. Zero Width/Height/Margin use A4
// and the legacy 54pt fallback. Margins and distance pointers retain explicit zeros.
type Layout = engine.Layout

const ListBlock = engine.ListBlock

// ListItem owns its ordered blocks, including continuation paragraphs and nested lists.
type ListItem = engine.ListItem

// ListMarker retains literal visible label content and its independent source owner.
// It does not define an automatic numbering or restart rule.
type ListMarker = engine.ListMarker

// ListNumbering describes an ordered-list label using OOXML's %1..%9
// placeholders. Level is zero-based. Formats contains one format per level;
// Ancestors contains fallback values for levels before Level. Enclosing lists
// override matching ancestor levels when serialized, so nested edits renumber.
// Legal renders every referenced level in decimal, retaining its original format.
type ListNumbering = engine.ListNumbering

// Matrix is [a b c d e f]: x'=a*x+c*y+e, y'=b*x+d*y+f.
type Matrix = engine.Matrix

type Metadata = engine.Metadata

type Note = engine.Note

// NoteReference identifies a note independently of its displayed automatic number.
// IDs are scoped by Kind and may be application identifiers rather than integers.
type NoteReference = engine.NoteReference

const PDF = engine.PDF

// PDFArtifact records an explicit marked-content artifact, independently of any
// inferred reusable header/footer template. BBox uses default user-space units (not the current content CTM); Bounds
// uses Matrix to map it into displayed physical points. Content refers back here
// through SourceInfo.Artifacts, including nested marked-content and images.
type PDFArtifact = engine.PDFArtifact

type PDFDestination = engine.PDFDestination

// PDFNavigation retains explicit PDF annotation/view evidence independently of
// inferred editable-flow anchors. Coordinates in Point/Region use displayed pt;
// Parameters retain original PDF user units and nullable viewer settings.
type PDFNavigation = engine.PDFNavigation

type PDFNavigationLink = engine.PDFNavigationLink

// PDFPage records physical page boundaries even when a logical tagged paragraph
// spans pages. Boxes retain PDF user units; Width/Height and source quads use pt.
type PDFPage = engine.PDFPage

const PageBreak = engine.PageBreak

// PageMargins overrides all four edges of Layout.Margin. Zero is an explicit
// zero margin. Top/Bottom may be negative for OOXML fixed body offsets that
// permit running-content overlap; Left/Right are nonnegative.
type PageMargins = engine.PageMargins

const Paragraph = engine.Paragraph
const Picture = engine.Picture

// Point is in physical points in displayed page coordinates: top-left origin,
// x right and y down. PDFPage.Transform maps original PDF user coordinates here.
type Point = engine.Point

type Quad = engine.Quad

// Read decodes a supported format into the shared document model.
func Read(ctx context.Context, r io.Reader, format Format, opts ReadOptions) (*Document, error) {
	return engine.Read(ctx, r, format, opts)
}

// ReadDOCX extracts supported DOCX content and representation-loss diagnostics.
func ReadDOCX(ctx context.Context, src io.Reader) (*Document, error) {
	return engine.ReadDOCX(ctx, src)
}

type PDFReadLimits = engine.PDFReadLimits

type ReadOptions = engine.ReadOptions

// ReadPDF recovers PDF content, physical source facts and qualified structure.
func ReadPDF(ctx context.Context, src io.Reader, opts ReadOptions) (doc *Document, err error) {
	return engine.ReadPDF(ctx, src, opts)
}

// ReadStructured reads and validates the ProseMirror profile.
func ReadStructured(ctx context.Context, r io.Reader) (*StructuredDocument, error) {
	return engine.ReadStructured(ctx, r)
}

// ReadVDOCS decodes the supported VDOCS native document envelope.
func ReadVDOCS(ctx context.Context, r io.Reader) (*Document, error) { return engine.ReadVDOCS(ctx, r) }

type Row = engine.Row

type Run = engine.Run

// RunningStories distinguishes an inherited nil story from an explicit empty one.
// Default serves odd pages (and all ordinary pages without EvenAndOddHeaders).
type RunningStories = engine.RunningStories

// RunningStory is an independently inherited header or footer variant.
// A non-nil story with no blocks explicitly suppresses that variant.
type RunningStory = engine.RunningStory

const SF = engine.SF
const SFGzip = engine.SFGzip

// Section owns body content. Layout nil inherits the previous effective layout
// (the document layout for the first section). Stories inherit independently.
// Native PDF keeps the stories of the section that opened a physical page.
// A continuous section counts a shared page as local page one, even when its
// body is empty. Later pages select its default/even stories using that count.
type Section = engine.Section

const SectionBlock = engine.SectionBlock
const SectionContinuous = engine.SectionContinuous
const SectionEvenPage = engine.SectionEvenPage
const SectionNextPage = engine.SectionNextPage
const SectionOddPage = engine.SectionOddPage

// SectionStart controls the section start relative to the preceding section.
// An empty value means nextPage; the first section starts the document.
type SectionStart = engine.SectionStart

// SourceElementReference retains an explicit logical cell that was an empty
// continuation of a geometrically recovered merged cell.
type SourceElementReference = engine.SourceElementReference

// SourceInfo records provenance, not an alternative copy of content. Quads use
// advance/font metrics, not glyph ink outlines. Precision discloses estimates.
// Confidence is a heuristic ranking, not a calibrated probability.
type SourceInfo = engine.SourceInfo

type SourceRegion = engine.SourceRegion

// SourceStructureReference identifies the explicit owner and targets of a PDF
// structure Ref entry. Aggregate source bounds retain contributing edges too.
type SourceStructureReference = engine.SourceStructureReference

type StructuredAttributes = engine.StructuredAttributes

// StructuredDocument is a ProseMirror document using the versioned schema in
// docs/reference/STRUCTURED_FORMAT.md. It needs no JavaScript or ProseMirror runtime.
type StructuredDocument = engine.StructuredDocument

type StructuredMark = engine.StructuredMark

// Attrs contains the attributes defined for this node type, encoded as an object.
// Unknown nodes, marks and attributes are rejected, never silently flattened.
type StructuredNode = engine.StructuredNode

type StructuredNote = engine.StructuredNote

const StructuredSchema = engine.StructuredSchema
const Table = engine.Table

// TableSpanSource separates explicit logical membership from recovered spans.
// ColSpan and RowSpan are "explicit" or "inferred". Bounds records the painted
// cell boundary; SourceInfo.Quad continues to record its content bounds.
type TableSpanSource = engine.TableSpanSource

// TableWidthSource records measured painted-grid widths independently from
// explicit PDF logical table membership. Confidence is not a probability.
type TableWidthSource = engine.TableWidthSource

const Text = engine.Text

// ToStructured converts the document model to its ProseMirror representation.
func ToStructured(d *Document) (*StructuredDocument, error) { return engine.ToStructured(d) }

const VDOCS = engine.VDOCS
const VDOCSMIME = engine.VDOCSMIME

// Write encodes the document, applying the selected loss and font policies.
func Write(ctx context.Context, w io.Writer, d *Document, format Format, opts WriteOptions) error {
	return engine.Write(ctx, w, d, format, opts)
}

type WriteOptions = engine.WriteOptions

// WriteStructured validates and writes the ProseMirror profile.
func WriteStructured(ctx context.Context, w io.Writer, s *StructuredDocument) error {
	return engine.WriteStructured(ctx, w, s)
}

// WriteVDOCS encodes the supported VDOCS native document envelope.
func WriteVDOCS(ctx context.Context, w io.Writer, d *Document) error {
	return engine.WriteVDOCS(ctx, w, d)
}

const PPTX = engine.PPTX

type Presentation = engine.Presentation
type PresentationSlide = engine.PresentationSlide
type PresentationResource = engine.PresentationResource
type SlideObject = engine.SlideObject
type StructuredPresentation = engine.StructuredPresentation

func ReadPPTX(ctx context.Context, r io.Reader) (*Document, error) { return engine.ReadPPTX(ctx, r) }

type StructuredSlide = engine.StructuredSlide
type StructuredSlideObject = engine.StructuredSlideObject
type StructuredSlideResource = engine.StructuredSlideResource
