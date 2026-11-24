package manydoc

import (
	"bytes"
	"context"
	"testing"
)

func TestDOCXPositionalTabContentAndLoss(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Left</w:t></w:r><w:r><w:ptab w:alignment="center" w:relativeTo="margin" w:leader="none"/></w:r><w:r><w:t>Middle</w:t></w:r><w:r><w:ptab w:alignment="right" w:relativeTo="margin" w:leader="dot"/></w:r><w:r><w:t>Right</w:t></w:r></w:p>`
	d, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Text() != "Left\tMiddle\tRight" || !hasDiagnostic(d, "position_tab") {
		t.Fatal("visible positional separation discarded or loss hidden", d.Text(), d.Diagnostics)
	}
	for _, format := range []Format{SF, DOCX} {
		var out bytes.Buffer
		if err := Write(context.Background(), &out, d, format, WriteOptions{AllowLossy: true}); err != nil {
			t.Fatal(err)
		}
		back, err := Read(context.Background(), &out, format, ReadOptions{})
		if err != nil || back.Text() != d.Text() {
			t.Fatal("normalized separator roundtrip", format, err)
		}
	}
	bad := `<w:p><w:r><w:ptab w:alignment="center" w:relativeTo="margin" w:leader="none"><w:t>Must not disappear</w:t></w:ptab></w:r></w:p>`
	if _, err := ReadDOCX(context.Background(), bytes.NewReader(docxWithBody(t, bad))); err == nil {
		t.Fatal("nonempty positional tab silently swallowed content")
	}
}
