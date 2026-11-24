package manydoc

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type pdfDestinationPoint struct {
	page int
	y    float64
}

func (r *pdfRenderer) prepareDestinations(d *Document) error {
	c, err := catalogDestinations(d, true)
	if err != nil {
		return err
	}
	if len(c.anchors)+len(c.links) == 0 {
		return nil
	}
	r.destinationPoints = map[string]pdfDestinationPoint{}
	r.destinationTokens = map[string]string{}
	r.destinationURIs = map[string]string{}
	used := map[string]bool{}
	if err := documentDestinationRuns(d, func(run Run) error {
		if run.Link != "" && run.Destination == "" {
			used[pdfASCIIURI(run.Link)] = true
		}
		return nil
	}); err != nil {
		return err
	}
	prefix := ""
	for nonce := 0; nonce <= len(used); nonce++ {
		candidate := fmt.Sprintf("urn:manydoc:internal:%d:", nonce)
		collision := false
		for uri := range used {
			if strings.HasPrefix(uri, candidate) {
				collision = true
				break
			}
		}
		if !collision {
			prefix = candidate
			break
		}
	}
	if prefix == "" {
		return fmt.Errorf("cannot allocate internal link namespace")
	}
	ids := make([]string, 0, len(c.links))
	for id := range c.links {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for i, id := range ids {
		token := fmt.Sprintf("%s%d", prefix, i)
		r.destinationTokens[token] = id
		r.destinationURIs[id] = token
	}
	return nil
}

func (r *pdfRenderer) bindDestinations(ids []string, y float64) {
	for _, id := range ids {
		if _, seen := r.destinationPoints[id]; seen {
			continue
		}
		if r.destinationPoints == nil {
			r.destinationPoints = map[string]pdfDestinationPoint{}
		}
		r.destinationPoints[id] = pdfDestinationPoint{r.pdf.PageNo(), y}
	}
}

// Bind only after shaping, line breaking and bidi placement. The anchor itself
// never enters text or changes graphemes, glyph context, advances or metrics.
// Navigation opens the containing line; it does not claim a glyph caret x.
func bindPDFLineAnchors(runs []Run, lines []pdfLine) []pdfLine {
	if len(lines) == 0 {
		lines = []pdfLine{{}}
	}
	offset := 0
	for _, run := range runs {
		if run.Anchor != "" {
			chosen := -1
			following := -1
			for li, line := range lines {
				for _, span := range line {
					if span.bidi.paragraph == nil {
						continue
					}
					if span.bidi.start <= offset && offset < span.bidi.end {
						chosen = li
						break
					}
					if following < 0 && span.bidi.start >= offset {
						following = li
					}
				}
				if chosen >= 0 {
					break
				}
			}
			if chosen < 0 {
				chosen = following
			}
			if chosen < 0 {
				chosen = len(lines) - 1
			}
			lines[chosen] = append(lines[chosen], pdfSpan{run: Run{Anchor: run.Anchor}})
		}
		if run.Note != nil {
			offset++
		} else {
			offset += utf8.RuneCountInString(run.Text)
		}
	}
	return lines
}

func pdfDestinationString(id string) types.HexLiteral {
	data := []byte{0xfe, 0xff}
	for _, unit := range utf16.Encode([]rune(id)) {
		data = append(data, byte(unit>>8), byte(unit))
	}
	return types.HexLiteral(hex.EncodeToString(data))
}

func (r *pdfRenderer) finishDestinations(p *model.Context) error {
	if len(r.destinationPoints)+len(r.destinationTokens) == 0 {
		return nil
	}
	ids := make([]string, 0, len(r.destinationPoints))
	for id := range r.destinationPoints {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return pdfDestinationString(ids[i]) < pdfDestinationString(ids[j]) })
	pairs := types.Array{}
	for _, id := range ids {
		point := r.destinationPoints[id]
		dict, ref, _, err := p.PageDict(point.page, false)
		if err != nil {
			return err
		}
		box, err := p.DereferenceArray(dict["MediaBox"])
		if err != nil {
			return err
		}
		// gofpdf may inherit the media box. Its canvas height is available in
		// the page's generated /MediaBox or through inherited attributes.
		if len(box) != 4 {
			_, _, inherited, e := p.PageDict(point.page, false)
			if e != nil {
				return e
			}
			if inherited.MediaBox == nil {
				return fmt.Errorf("destination page lacks media box")
			}
			box = types.Array{types.Float(inherited.MediaBox.LL.X), types.Float(inherited.MediaBox.LL.Y), types.Float(inherited.MediaBox.UR.X), types.Float(inherited.MediaBox.UR.Y)}
		}
		top, err := pdfNumber(box[3])
		if err != nil {
			return err
		}
		pairs = append(pairs, pdfDestinationString(id), types.Array{*ref, types.Name("XYZ"), types.Integer(0), types.Float(top - point.y), nil})
	}
	if len(pairs) > 0 {
		catalog, err := p.Catalog()
		if err != nil {
			return err
		}
		names, err := p.DereferenceDict(catalog["Names"])
		if err != nil {
			return err
		}
		if names == nil {
			names = types.Dict{}
		}
		if names["Dests"] != nil {
			return fmt.Errorf("unexpected generated destination name tree")
		}
		names["Dests"] = types.Dict{"Names": pairs}
		catalog["Names"] = names
	}
	changed := 0
	for page := 1; page <= p.PageCount; page++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		dict, _, _, err := p.PageDict(page, false)
		if err != nil {
			return err
		}
		annots, err := p.DereferenceArray(dict["Annots"])
		if err != nil {
			return err
		}
		for _, ref := range annots {
			a, err := p.DereferenceDict(ref)
			if err != nil {
				return err
			}
			action, err := p.DereferenceDict(a["A"])
			if err != nil {
				return err
			}
			if action == nil {
				continue
			}
			kind := action.NameEntry("S")
			if kind == nil || *kind != "URI" {
				continue
			}
			uri, err := pdfTextString(action["URI"])
			if err != nil {
				return err
			}
			id, ok := r.destinationTokens[uri]
			if !ok {
				continue
			}
			a["A"] = types.Dict{"S": types.Name("GoTo"), "D": pdfDestinationString(id)}
			changed++
		}
	}
	if len(r.destinationTokens) > 0 && changed == 0 {
		return fmt.Errorf("internal link annotations were not serialized")
	}
	return nil
}
