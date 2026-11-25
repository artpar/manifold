package engine

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/hhrutter/lzw"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	// pdfTrailerAt only inspects unencrypted file structure. A failed discovery leaves
	// the original input to the ordinary parser, including its existing repair rules.
	"golang.org/x/image/ccitt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"sort"
	"strconv"
)

func pdfTrailerAt(ctx context.Context, b []byte, offset int) (types.Dict, int, error) {
	if offset < 0 || offset >= len(b) {
		return nil, 0, nil
	}
	end := offset + min(maxPDFStreamBytes, len(b)-offset)
	text := string(b[offset:end])
	next := func() (any, error) { return nextPDFToken(ctx, &text) }
	first, err := next()
	if err != nil {
		return nil, 0, err
	}
	high := 0
	finish := func(d types.Dict) (types.Dict, int, error) {
		if d == nil {
			return nil, 0, nil
		}
		if size, ok := d["Size"].(types.Integer); ok && size > 0 {
			high = max(high, int(size))
		}
		if index, ok := d["Index"].(types.Array); ok {
			if len(index)%2 != 0 {
				return nil, 0, nil
			}
			for i := 0; i < len(index); i += 2 {
				start, a := index[i].(types.Integer)
				count, c := index[i+1].(types.Integer)
				if !a || !c || start < 0 || count < 0 || int(start) > int(^uint(0)>>1)-int(count) {
					return nil, 0, nil
				}
				high = max(high, int(start)+int(count))
			}
		}
		return d, high, nil
	}
	if first == pdfOperator("xref") {
		for text != "" {
			token, err := next()
			if err != nil {
				return nil, 0, err
			}
			if token == pdfOperator("trailer") {
				object, err := model.ParseObjectContext(ctx, &text)
				if err != nil {
					return nil, 0, err
				}
				d, _ := object.(types.Dict)
				return finish(d)
			}
			start, ok := token.(types.Integer)
			if !ok || start < 0 {
				return nil, 0, nil
			}
			token, err = next()
			if err != nil {
				return nil, 0, err
			}
			count, ok := token.(types.Integer)
			if !ok || count < 0 || int(start) > int(^uint(0)>>1)-int(count) {
				return nil, 0, nil
			}
			high = max(high, int(start)+int(count))
			for i := 0; i < int(count); i++ {
				for field := 0; field < 3; field++ {
					token, err := next()
					if err != nil {
						return nil, 0, err
					}
					if token == nil {
						return nil, 0, nil
					}
				}
			}
		}
		return nil, 0, nil
	}
	if _, ok := first.(types.Integer); !ok {
		return nil, 0, nil
	}
	gen, err := next()
	if err != nil {
		return nil, 0, err
	}
	if _, ok := gen.(types.Integer); !ok {
		return nil, 0, nil
	}
	keyword, err := next()
	if err != nil {
		return nil, 0, err
	}
	if keyword != pdfOperator("obj") {
		return nil, 0, nil
	}
	object, err := model.ParseObjectContext(ctx, &text)
	if err != nil {
		return nil, 0, err
	}
	d, _ := object.(types.Dict)
	if d == nil || d["Type"] != types.Name("XRef") {
		return nil, 0, nil
	}
	return finish(d)
}

// adaptDirectPDFEncryption adds an in-memory incremental trailer which gives a
// direct encryption dictionary an indirect identity understood by pdfcpu. Original
// objects, ciphertext, relative byte offsets and file IDs remain intact. A
// non-PDF header prefix is excluded from this parser view. Password checking
// and decryption still run through the existing Go security handler.

// Decode byte filters before invoking a raster renderer. Terminal JPEG headers
// are checked before any consumer can expand them; fax decoding is bounded too.
// No cached syntax entry is used for a partial image-filter pipeline.

func adaptDirectPDFEncryption(ctx context.Context, b []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	marker := bytes.LastIndex(b, []byte("startxref"))
	if marker < 0 {
		return b, nil
	}
	position := marker + len("startxref")
	probe := &pdfReader{ctx: ctx}
	start, err := probe.headerInteger(b, &position)
	if err != nil || start < 0 || start >= len(b) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return b, nil
	}
	header := bytes.Index(b, []byte("%PDF-"))
	if header < 0 || header > 1024 {
		return b, nil
	}
	visited := map[int]bool{}
	var latest types.Dict
	var encryption types.Dict
	offset := start
	work := 0
	fresh := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if visited[offset] || offset < 0 || offset >= len(b)-header {
			return b, nil
		}
		visited[offset] = true
		work += min(maxPDFStreamBytes, len(b)-offset-header)
		if work > maxPDFDecodedBytes {
			return b, nil
		}
		d, high, e := pdfTrailerAt(ctx, b, offset+header)
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return b, nil
		}
		if d == nil {
			return b, nil
		}
		fresh = max(fresh, high)
		if latest == nil {
			latest = types.Dict{}
			for k, v := range d {
				latest[k] = v
			}
		}
		for _, key := range []string{"Root", "ID", "Info"} {
			if latest[key] == nil && d[key] != nil {
				latest[key] = d[key]
			}
		}
		if encryption == nil && d["Encrypt"] != nil {
			encryption, _ = d["Encrypt"].(types.Dict)
			if encryption == nil {
				return b, nil
			}
		}

		prev, ok := d["Prev"].(types.Integer)
		if !ok {
			break
		}
		offset = int(prev)
	}
	if encryption == nil {
		return b, nil
	}
	size, ok := latest["Size"].(types.Integer)
	if ok {
		size = types.Integer(max(int(size), fresh))
	}
	root, rootOK := latest["Root"].(types.IndirectRef)
	id, idOK := latest["ID"].(types.Array)
	if !ok || size <= 0 || int(size) == int(^uint(0)>>

		// Device samples are read by component, including row padding and16-bit values.
		// Keeping high-depth samples in NRGBA64 avoids silently dropping their low byte.

		// validateParserStreams verifies the original decoded source, not the dependency's
		// possibly lenient cached Content. It runs before page access. The dependency has
		// already read xref streams and object-stream prologs: this is not a guard against
		// allocations made inside ReadWithContext itself.

		// Object-stream prologs contain integer pairs, with PDF whitespace and comments.
		// Scan in place instead of allocating a field for every whitespace separator.

		// Internal decoded-work/allocation bounds, independent of application quotas.

		1) || !rootOK || root.ObjectNumber <= 0 || root.ObjectNumber >= size || !idOK || len(id) != 2 {
		return nil, fmt.Errorf("invalid direct PDF encryption trailer")
	}
	for _, value := range id {
		if _, err := pdfString(value); err != nil {
			return nil, fmt.Errorf("invalid direct PDF encryption file ID")
		}
	}
	for _, key := range []string{"Type", "W", "Index", "Length", "Filter", "DecodeParms", "DL", "XRefStm"} {
		delete(latest, key)
	}
	latest["Size"] = size + 1
	latest["Encrypt"] = *types.NewIndirectRef(int(size), 0)
	latest["Prev"] = types.Integer(start)
	var extra bytes.Buffer
	extra.WriteByte('\n')
	objectOffset := len(b) + extra.Len() - header
	if int64(objectOffset) > 9_999_999_999 {
		return nil, fmt.Errorf("PDF encryption adapter xref offset exceeds supported range")
	}
	fmt.Fprintf(&extra, "%d 0 obj\n%s\nendobj\n", size, encryption.PDFString())
	xrefOffset := len(b) + extra.Len() - header
	fmt.Fprintf(&extra, "xref\n%d 1\n%010d 00000 n \ntrailer\n%s\nstartxref\n%d\n%%%%EOF\n", size, objectOffset, latest.PDFString(), xrefOffset)
	out := make([]byte, 0, len(b)-header+extra.Len())
	out = append(out, b[header:]...)
	out = append(out, extra.Bytes()...)
	return out, nil
}

const maxPDFImageBytes = 200_000_000
const maxPDFRasterPixels = int64(100_000_000)

func (r *pdfReader) chargePDFRaster(w, h int) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	n := int64(w) * int64(h)
	if n > maxPDFRasterPixels-r.rasterPixels {
		return fmt.Errorf("PDF raster work exceeds %d pixels", maxPDFRasterPixels)
	}
	r.rasterPixels += n
	return nil
}

func (r *pdfReader) imageBoolParam(d types.Dict, key string, fallback bool) (bool, error) {
	if d[key] == nil {
		return fallback, nil
	}
	v, err := r.pdf.Dereference(d[key])
	if err != nil {
		return false, err
	}
	if b, ok := v.(types.Boolean); ok {
		return bool(b), nil
	}
	return false, fmt.Errorf("invalid PDF image boolean %s", key)
}

func (r *pdfReader) preparePDFImage(sd *types.StreamDict, w, h int) error {
	if err := r.chargePDFRaster(w, h); err != nil {
		return err
	}
	mask, err := r.imageBoolParam(sd.Dict, "ImageMask", false)
	if err != nil {
		return err
	}
	bits, err := r.pdfStreamParam(sd.Dict, "BitsPerComponent", 8)
	if err != nil {
		return err
	}
	if mask {
		bits = 1
	}
	if sd.Dict["ImageMask"] != nil {
		sd.Dict["ImageMask"] = types.Boolean(mask)
	}
	if bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16 {
		return fmt.Errorf("invalid PDF image sample depth")
	}
	sd.Dict["BitsPerComponent"] = types.Integer(bits)
	filters := sd.FilterPipeline
	if len(filters) > maxPDFStreamFilters {
		return fmt.Errorf("PDF stream exceeds %d filters", maxPDFStreamFilters)
	}
	last := ""
	if len(filters) > 0 {
		last = filters[len(filters)-1].Name
	}
	if last == "DCTDecode" && bits != 8 || last == "CCITTFaxDecode" && bits != 1 {
		return fmt.Errorf("PDF image sample depth contradicts terminal codec")
	}
	prefix := *sd
	terminal := last == "DCTDecode" || last == "CCITTFaxDecode"
	if terminal {
		prefix.FilterPipeline = filters[:len(filters)-1]
		prefix.Content = nil
	}
	if last == "JPXDecode" || last == "JBIG2Decode" {
		return fmt.Errorf("%w: PDF image codec %s", ErrUnsupported, last)
	}
	data, err := r.decodePDFStreamLimit(nil, &prefix, maxPDFImageBytes)
	if err != nil {
		return err
	}
	if last == "DCTDecode" {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("invalid PDF JPEG header: %w", err)
		}
		if cfg.Width != w || cfg.Height != h {
			return fmt.Errorf("image stream dimensions contradict image dictionary")
		}
		sd.Raw = data
		sd.Content = nil
		sd.FilterPipeline = filters[len(filters)-1:]
		return nil
	}
	if last == "CCITTFaxDecode" {
		parms := filters[len(filters)-1].DecodeParms
		columns, err := r.pdfStreamParam(parms, "Columns", 1728)
		if err != nil {
			return err
		}
		rows, err := r.pdfStreamParam(parms, "Rows", h)
		if err != nil {
			return err
		}
		if rows == 0 {
			rows = h
		}
		if columns != w || rows != h {
			return fmt.Errorf("CCITT parameters contradict image dimensions")
		}
		k, err := r.pdfStreamParam(parms, "K", 0)
		if err != nil {
			return err
		}
		if k > 0 {
			return fmt.Errorf("%w: mixed CCITT Group3 encoding", ErrUnsupported)
		}
		invert, err := r.imageBoolParam(parms, "BlackIs1", false)
		if err != nil {
			return err
		}
		align, err := r.imageBoolParam(parms, "EncodedByteAlign", false)
		if err != nil {
			return err
		}
		mode := ccitt.Group3
		if k < 0 {
			mode = ccitt.Group4
		}
		reader := ccitt.NewReader(contextReader{r.ctx, bytes.NewReader(data)}, ccitt.MSB, mode, w, h, &ccitt.Options{Invert: invert, Align: align})
		limit := min(maxPDFImageBytes, maxPDFDecodedBytes-r.streamBudget.total)
		data, err = readPDFDecoded(r.ctx, reader, limit)
		if err != nil {
			return err
		}
		r.streamBudget.total += len(data)
		if sd.Dict["ColorSpace"] == nil {
			sd.Dict["ColorSpace"] = types.Name("DeviceGray")
		}
		bits = 1
		sd.Dict["BitsPerComponent"] = types.Integer(1)
	}
	sd.Raw = data
	sd.Content = nil
	sd.FilterPipeline = nil
	return nil
}

func (r *pdfReader) preparePDFColorSpace(o types.Object, depth int) (types.Object, error) {
	if depth > 16 {
		return nil, fmt.Errorf("image color space nesting exceeds limit")
	}
	v, err := r.pdf.Dereference(o)
	if err != nil {
		return nil, err
	}
	a, ok := v.(types.Array)
	if !ok {
		return v, nil
	}
	a = append(types.Array(nil), a...)
	if len(a) == 4 && a[0] == types.Name("Indexed") {
		a[1], err = r.preparePDFColorSpace(a[1], depth+1)
		if err != nil {
			return nil, err
		}
		lookup, err := r.pdf.Dereference(a[3])
		if err != nil {
			return nil, err
		}
		if sd, ok := lookup.(types.StreamDict); ok {
			data, err := r.decodePDFStream(a[3], &sd)
			if err != nil {
				return nil, err
			}
			a[3] = types.HexLiteral(hex.EncodeToString(data))
		}
	}
	return a, nil
}

func (r *pdfReader) deviceImage(sd *types.StreamDict, w, h int) (image.Image, bool, error) {
	if len(sd.FilterPipeline) != 0 {
		return nil, false, nil
	}
	name, ok := sd.Dict["ColorSpace"].(types.Name)
	if !ok {
		return nil, false, nil
	}
	bits := int(*sd.IntEntry("BitsPerComponent"))
	channels := 0
	switch name {
	case "DeviceGray":
		channels = 1
	case "DeviceRGB":
		channels = 3
	case "DeviceCMYK":
		channels = 4
		r.warnPage("pdf_image_color", "Device CMYK samples are converted to RGB; original device-color semantics are not retained")
	default:
		return nil, false, nil
	}
	row := (w*channels*bits + 7) / 8
	if len(sd.Raw) < row*h {
		return nil, true, fmt.Errorf("truncated PDF image samples")
	}
	decode := make([]float64, channels*2)
	for c := 0; c < channels; c++ {
		decode[2*c+1] = 1
	}
	if d, err := r.pdf.DereferenceArray(sd.Dict["Decode"]); err != nil {
		return nil, true, err
	} else if len(d) > 0 {
		if len(d) != len(decode) {
			return nil, true, fmt.Errorf("invalid PDF image Decode array")
		}
		for i, v := range d {
			decode[i], err = r.pdf.DereferenceNumber(v)
			if err != nil || !finite(decode[i]) {
				return nil, true, fmt.Errorf("invalid PDF image Decode value")
			}
		}
	}
	sample := func(y, index int) uint16 {
		start := index * bits
		var value uint32
		for bit := 0; bit < bits; bit++ {
			n := start + bit
			value = value<<1 | uint32(sd.Raw[y*row+n/8]>>uint(7-n%8)&1)
		}
		c := index % channels
		fraction := float64(value) / float64((uint32(1)<<uint(bits))-1)
		normalized := (1-fraction)*decode[2*c] + fraction*decode[2*c+1]
		return uint16(max(0, min(1, normalized))*65535 + .5)
	}
	colorSample := func(y, x int) (uint16, uint16, uint16) {
		a := sample(y, x*channels)
		if channels == 1 {
			return a, a, a
		}
		b, c := sample(y, x*channels+1), sample(y, x*channels+2)
		if channels == 4 {
			k := uint32(65535 - sample(y, x*channels+3))
			return uint16(uint32(65535-a) * k / 65535), uint16(uint32(65535-b) * k / 65535), uint16(uint32(65535-c) * k / 65535)
		}
		return a, b, c
	}
	if bits == 16 {
		dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			if err := r.ctx.Err(); err != nil {
				return nil, true, err
			}
			for x := 0; x < w; x++ {
				red, green, blue := colorSample(y, x)
				dst.SetNRGBA64(x, y, color.NRGBA64{R: red, G: green, B: blue, A: 65535})
			}
		}
		return dst,

			// PDF content arrays concatenate bytes; do not invent token separators.
			true, nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		if err := r.ctx.Err(); err != nil {
			return nil, true, err
		}
		for x := 0; x < w; x++ {
			r, g, b := colorSample(y, x)
			red, green, blue := uint8(r>>8), uint8(g>>8), uint8(b>>8)
			dst.SetNRGBA(x, y, color.NRGBA{R: red, G: green, B: blue, A: 255})
		}
	}
	return dst, true, nil
}

func (r *pdfReader) validateParserStreams() error {
	ids := make([]int, 0, len(r.pdf.Table))
	owners := map[int]map[int]int{}
	for id, entry := range r.pdf.Table {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if entry == nil || entry.Free {
			continue
		}
		ids = append(ids, id)
		if entry.ObjectStream != nil && entry.ObjectStreamInd != nil {
			stream, index := *entry.ObjectStream, *entry.ObjectStreamInd
			if owners[stream] == nil {
				owners[stream] = map[int]int{}
			}
			if previous, found := owners[stream][index]; found && previous != id {
				return fmt.Errorf("PDF object stream %d index %d has conflicting owners", stream, index)
			}
			owners[stream][index] = id
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		var sd types.StreamDict
		objectStream := false
		switch object := r.pdf.Table[id].Object.(type) {
		case types.ObjectStreamDict:
			sd = object.StreamDict
			objectStream = true
		case types.XRefStreamDict:
			sd = object.StreamDict
		case types.StreamDict:
			if object.IsObjStm() {
				sd = object
				objectStream = true
			} else if typ := object.NameEntry("Type"); typ != nil && *typ == "XRef" {
				sd = object
			} else {
				continue
			}
		default:
			continue
		}
		cached := sd.Content
		sd.Content = nil
		decoded, err := r.decodePDFStream(nil, &sd)
		if err != nil {
			return fmt.Errorf("PDF parser stream %d: %w", id, err)
		}
		if cached != nil && !bytes.Equal(cached, decoded) {
			return fmt.Errorf("PDF parser stream %d decoded source disagrees with parser content", id)
		}
		if objectStream {
			if err := r.validateObjectStreamHeader(id, &sd, decoded, owners[id]); err != nil {
				return err
			}
			delete(owners, id)
		}
	}
	if len(owners) > 0 {
		return fmt.Errorf("PDF compressed objects have no validated object stream")
	}
	return nil
}

func pdfHeaderSpace(c byte) bool {
	return c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == ' '
}

func (r *pdfReader) skipHeaderSpace(header []byte, pos *int) error {
	for *pos < len(header) {
		if *pos%8192 == 0 {
			if err := r.ctx.Err(); err != nil {
				return err
			}
		}
		c := header[*pos]
		if pdfHeaderSpace(c) {
			*pos++
			continue
		}
		if c != '%' {
			break
		}
		for *pos < len(header) && header[*pos] != '\r' && header[*pos] != '\n' {
			if *pos%8192 == 0 {
				if err := r.ctx.Err(); err != nil {
					return err
				}
			}
			*pos++
		}
	}
	return nil
}
func (r *pdfReader) headerInteger(header []byte, pos *int) (int, error) {
	if err := r.skipHeaderSpace(header, pos); err != nil {
		return 0, err
	}
	start := *pos
	for *pos < len(header) && !pdfHeaderSpace(header[*pos]) && header[*pos] != '%' {
		if *pos%8192 == 0 {
			if err := r.ctx.Err(); err != nil {
				return 0, err
			}
		}
		*pos++
	}
	if start == *pos {
		return 0, fmt.Errorf("missing object-stream header integer")
	}
	n, err := strconv.Atoi(string(header[start:*pos]))
	if err != nil {
		return 0, fmt.Errorf("invalid object-stream header integer")
	}
	return n, nil
}
func (r *pdfReader) validateObjectStreamHeader(id int, sd *types.StreamDict, data []byte, owners map[int]int) error {
	n, err := r.pdfStreamParam(sd.Dict, "N", -1)
	if err != nil {
		return err
	}
	first, err := r.pdfStreamParam(sd.Dict, "First", -1)
	if err != nil {
		return err
	}
	if n < 0 || first < 0 || first > len(data) {
		return fmt.Errorf("PDF object stream %d has invalid N/First", id)
	}
	for index := range owners {
		if index < 0 || index >= n {
			return fmt.Errorf("PDF object stream %d compressed index is outside N", id)
		}
	}
	header := data[:first]
	pos, previous := 0, -1
	seen := map[int]bool{}
	for index := 0; index < n; index++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		member, err := r.headerInteger(header, &pos)
		if err != nil {
			return fmt.Errorf("PDF object stream %d: %w", id, err)
		}
		offset, err := r.headerInteger(header, &pos)
		if err != nil {
			return fmt.Errorf("PDF object stream %d: %w", id, err)
		}
		if member <= 0 || seen[member] || offset < 0 || offset <= previous || offset >= len(data)-first {
			return fmt.Errorf("PDF object stream %d has invalid member identity/offset", id)
		}
		seen[member] = true
		previous = offset
		if owner, found := owners[index]; found && owner != member {
			return fmt.Errorf("PDF object stream %d index %d header owner %d contradicts xref owner %d", id, index, member, owner)
		}
	}
	if err := r.skipHeaderSpace(header, &pos); err != nil {
		return err
	}
	if pos != len(header) {
		return fmt.Errorf("PDF object stream %d header count contradicts N", id)
	}
	return nil
}

const maxPDFStreamBytes = 64 << 20
const maxPDFDecodedBytes = 256 << 20
const maxPDFStreamFilters = 16

var errPDFDecodedLimit = errors.New("PDF decoded stream limit exceeded")

type pdfStreamBudget struct {
	total int
	cache map[types.IndirectRef][]byte
}

func readPDFDecoded(ctx context.Context, src io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, src}, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
	}
	return data, nil
}

func (r *pdfReader) decodePDFStream(object types.Object, sd *types.StreamDict) ([]byte, error) {
	return r.decodePDFStreamLimit(object, sd, maxPDFStreamBytes)
}

func (r *pdfReader) decodePDFStreamLimit(object types.Object, sd *types.StreamDict, streamLimit int) ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.streamBudget == nil {
		r.streamBudget = &pdfStreamBudget{cache: map[types.IndirectRef][]byte{}}
	}
	ref, indirect := object.(types.IndirectRef)
	if data, ok := r.streamBudget.cache[ref]; indirect && ok {
		return data, nil
	}
	charge := func(data []byte) error {
		if len(data) > streamLimit {
			return fmt.Errorf("PDF decoded stream exceeds %d bytes", streamLimit)
		}
		if len(data) > maxPDFDecodedBytes-r.streamBudget.total {
			return fmt.Errorf("PDF decoded streams exceed %d bytes", maxPDFDecodedBytes)
		}
		r.streamBudget.total += len(data)
		return nil
	}
	if len(sd.FilterPipeline) > maxPDFStreamFilters {
		return nil, fmt.Errorf("PDF stream exceeds %d filters", maxPDFStreamFilters)
	}
	data := sd.Content
	if data == nil {
		data = sd.Raw
		if len(sd.FilterPipeline) == 0 {
			if err := charge(data); err != nil {
				return nil, err
			}
		}
		for _, f := range sd.FilterPipeline {
			limit := min(streamLimit, maxPDFDecodedBytes-r.streamBudget.total)
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
			var err error
			switch f.Name {
			case "FlateDecode":
				var reader io.ReadCloser
				reader, err = zlib.NewReader(contextReader{r.ctx, bytes.NewReader(data)})
				if err == nil {
					data, err = readPDFDecoded(r.ctx, reader, limit)
					closeErr := reader.Close()
					if err == nil {
						err = closeErr
					}
				}
			case "LZWDecode":
				early, e := r.pdfStreamParam(f.DecodeParms, "EarlyChange", 1)
				if e != nil {
					return nil, e
				}
				if early != 0 && early != 1 {
					return nil, fmt.Errorf("invalid PDF LZW EarlyChange")
				}
				reader := lzw.NewReader(contextReader{r.ctx, bytes.NewReader(data)}, early == 1)
				data, err = readPDFDecoded(r.ctx, reader, limit)
				closeErr := reader.Close()
				if err == nil {
					err = closeErr
				}
			case "ASCII85Decode":
				end := bytes.Index(data, []byte("~>"))
				if end < 0 {
					return nil, fmt.Errorf("PDF ASCII85 stream lacks EOD")
				}
				data, err = readPDFDecoded(r.ctx, ascii85.NewDecoder(bytes.NewReader(data[:end])), limit)
			case "ASCIIHexDecode":
				data, err = decodePDFHex(r.ctx, data, limit)
			case "RunLengthDecode":
				data, err = decodePDFRunLength(r.ctx, data, limit)
			default:
				return nil, fmt.Errorf("%w: PDF syntax stream filter %q", ErrUnsupported, f.Name)
			}
			if err != nil {
				if errors.Is(err, errPDFDecodedLimit) && limit < streamLimit {
					return nil, fmt.Errorf("PDF decoded streams exceed %d bytes: %w", maxPDFDecodedBytes, err)
				}
				return nil, fmt.Errorf("PDF stream %s: %w", f.Name, err)
			}
			if err = charge(data); err != nil {
				return nil, err
			}
			if f.Name == "FlateDecode" || f.Name == "LZWDecode" {
				predictor, e := r.pdfStreamParam(f.DecodeParms, "Predictor", 1)
				if e != nil {
					return nil, e
				}
				if predictor != 1 {
					colors, e := r.pdfStreamParam(f.DecodeParms, "Colors", 1)
					if e != nil {
						return nil, e
					}
					columns, e := r.pdfStreamParam(f.DecodeParms, "Columns", 1)
					if e != nil {
						return nil, e
					}
					bits, e := r.pdfStreamParam(f.DecodeParms, "BitsPerComponent", 8)
					if e != nil {
						return nil, e
					}
					data, err = unpredictPDFLimit(r.ctx, data, predictor, colors, columns, bits, streamLimit)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	} else if err := charge(data); err != nil {
		return nil, err
	}
	if indirect {
		r.streamBudget.cache[ref] = data
	}
	return data, nil
}

func (r *pdfReader) pdfStreamParam(d types.Dict, name string, fallback int) (int, error) {
	value := d[name]
	if value == nil {
		return fallback, nil
	}
	if n, ok := value.(types.Integer); ok {
		return int(n), nil
	}
	if r.pdf != nil {
		n, err := r.pdf.DereferenceInteger(value)
		if err == nil && n != nil {
			return int(*n), nil
		}
	}
	return 0, fmt.Errorf("invalid PDF stream parameter %s", name)
}

func decodePDFHex(ctx context.Context, data []byte, limit int) ([]byte, error) {
	var out []byte
	pending := -1
	for i, c := range data {
		if i%8192 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if c == '>' {
			break
		}
		if pdfSpace(c) {
			continue
		}
		value := -1
		switch {
		case c >= '0' && c <= '9':
			value = int(c - '0')
		case c >= 'a' && c <= 'f':
			value = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			value = int(c-'A') + 10
		}
		if value < 0 {
			return nil, fmt.Errorf("invalid PDF ASCIIHex digit")
		}
		if pending < 0 {
			pending = value
			continue
		}
		if len(out) >= limit {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		out = append(out, byte(pending<<4|value))
		pending = -1
	}
	if pending >= 0 {
		if len(out) >= limit {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		out = append(out, byte(pending<<4))
	}
	return out, nil
}

func decodePDFRunLength(ctx context.Context, data []byte, limit int) ([]byte, error) {
	var out []byte
	for i := 0; i < len(data); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := int(data[i])
		i++
		if n == 128 {
			return out, nil
		}
		size := n + 1
		if n > 128 {
			size = 257 - n
		}
		if size > limit-len(out) {
			return nil, fmt.Errorf("%w: %d bytes", errPDFDecodedLimit, limit)
		}
		if n < 128 {
			if size > len(data)-i {
				return nil, fmt.Errorf("truncated PDF RunLength literal")
			}
			out = append(out, data[i:i+size]...)
			i += size
		} else {
			if i >= len(data) {
				return nil, fmt.Errorf("truncated PDF RunLength repeat")
			}
			for j := 0; j < size; j++ {
				out = append(out, data[i])
			}
			i++
		}
	}
	return nil, fmt.Errorf("PDF RunLength stream lacks EOD")
}

func (r *pdfReader) pageContent(d types.Dict) ([]byte, error) {
	object := d["Contents"]
	if object == nil {
		return nil, nil
	}
	resolved, err := r.pdf.Dereference(object)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, nil
	}
	objects := types.Array{object}
	if a, ok := resolved.(types.Array); ok {
		objects = a
	}
	var result []byte
	for _, o := range objects {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		if o == nil {
			continue
		}
		sd, _, err := r.pdf.DereferenceStreamDict(o)
		if err != nil {
			return nil, err
		}
		if sd == nil {
			continue
		}
		data, err := r.decodePDFStream(o, sd)
		if err != nil {
			return nil, err
		}
		if len(data) > maxPDFStreamBytes-len(result) {
			return nil, fmt.Errorf("PDF page content exceeds %d bytes", maxPDFStreamBytes)
		}

		result = append(result, data...)
	}
	return result, nil
}
func unpredictPDF(ctx context.Context, data []byte, predictor, colors, columns, bits int) ([]byte, error) {
	return unpredictPDFLimit(ctx, data, predictor, colors, columns, bits, maxPDFStreamBytes)
}

func unpredictPDFLimit(ctx context.Context, data []byte, predictor, colors, columns, bits, limit int) ([]byte, error) {
	if predictor != 2 && (predictor < 10 || predictor > 15) {
		return nil, fmt.Errorf("%w: PDF stream predictor %d", ErrUnsupported, predictor)
	}
	if colors < 1 || columns < 1 || (bits != 1 && bits != 2 && bits != 4 && bits != 8 && bits != 16) {
		return nil, fmt.Errorf("invalid PDF predictor dimensions/sample depth")
	}
	if int64(colors) > int64(limit)*8/int64(bits)/int64(columns) {
		return nil, fmt.Errorf("PDF predictor row exceeds decoded stream limit")
	}
	samples := colors * columns
	rowBytes := (samples*bits + 7) / 8
	pixelBytes := (colors*bits + 7) / 8
	if predictor == 2 {
		if len(data)%rowBytes != 0 {
			return nil, fmt.Errorf("truncated PDF TIFF predictor row")
		}
		mask := uint32(1<<bits) - 1
		sample := func(row []byte, index int) uint32 {
			var v uint32
			for j := 0; j < bits; j++ {
				bit := index*bits + j
				v = v<<1 | uint32(row[bit/8]>>uint(7-bit%8)&1)
			}
			return v
		}
		for offset := 0; offset < len(data); offset += rowBytes {
			row := data[offset : offset+rowBytes]
			for i := colors; i < samples; i++ {
				if i%4096 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				value := (sample(row, i) + sample(row, i-colors)) & mask
				for j := 0; j < bits; j++ {
					bit := i*bits + j
					shift := uint(7 - bit%8)
					row[bit/8] = row[bit/8]&^(1<<shift) | byte(value>>uint(bits-j-1)&1)<<shift
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		return data, nil
	}
	stride := rowBytes + 1
	if len(data)%stride != 0 {
		return nil, fmt.Errorf("truncated PDF PNG predictor row")
	}
	out := make([]byte, len(data)/stride*rowBytes)
	for src, dst := 0, 0; src < len(data); src, dst = src+stride, dst+rowBytes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filter := data[src]
		if filter > 4 {
			return nil, fmt.Errorf("invalid PDF PNG predictor filter")
		}
		for i := 0; i < rowBytes; i++ {
			if i%8192 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			var left, up, upperLeft byte
			if i >= pixelBytes {
				left = out[dst+i-pixelBytes]
			}
			if dst > 0 {
				up = out[dst+i-rowBytes]
				if i >= pixelBytes {
					upperLeft = out[dst+i-rowBytes-pixelBytes]
				}
			}
			v := data[src+1+i]
			switch filter {
			case 1:
				v += left
			case 2:
				v += up
			case 3:
				v += byte((int(left) + int(up)) / 2)
			case 4:
				v += pdfPaeth(left, up, upperLeft)
			}
			out[dst+i] = v
		}
	}
	return out, nil
}
func pdfPaeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	if pa <= pb && pa <= pc {
		return a
	}
	if pb <= pc {
		return b
	}
	return c
}
