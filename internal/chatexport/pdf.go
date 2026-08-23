package chatexport

import (
	"context"
	"io"
	"regexp"
	"strconv"

	pdflib "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/util"
)

// bookmarkSentinelRe matches the <!--export-bookmark:N--> sentinel comment
// ToMarkdown emits immediately before each heading.
var bookmarkSentinelRe = regexp.MustCompile(`<!--export-bookmark:(\d+)-->`)

// bookmarkFpdf wraps pdflib.Fpdf to expose gofpdf's native Bookmark method.
// pdflib.Fpdf holds its *gofpdf.Fpdf as a field explicitly named "Fpdf"
// (not an embedded/anonymous field), so that method is never promoted onto
// pdflib.Fpdf itself - a plain type assertion for Bookmark on it always
// fails silently. This mirrors
// github.com/shrsv/AgentLaws/internal/renderer/pdf/fixed_fpdf.go's
// identical fixedFpdf.Bookmark, confirmed against the same library version.
type bookmarkFpdf struct {
	*pdflib.Fpdf
}

func (f *bookmarkFpdf) Bookmark(text string, level int, y float64) {
	f.Fpdf.Fpdf.Bookmark(text, level, y)
}

// bookmarkNodeRenderer registers each sentinel as a PDF /Outlines entry via
// the underlying *gofpdf.Fpdf's native Bookmark(text, level, y) method,
// which goldmark-pdf's own PDF interface doesn't expose but promotes
// through the concrete *pdflib.Fpdf passed to WithPDF - reached here by a
// type assertion, the same pattern
// github.com/shrsv/AgentLaws/internal/renderer/pdf uses for the same
// library.
//
// A standalone-line HTML comment parses as a block-level ast.KindHTMLBlock
// node; goldmark-pdf's default renderer for that kind is a no-op ("Cannot
// process HTML blocks"), so without this override the sentinel would be
// silently dropped.
type bookmarkNodeRenderer struct {
	bookmarks []BookmarkEntry
}

func (r *bookmarkNodeRenderer) RegisterFuncs(reg pdflib.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
}

func (r *bookmarkNodeRenderer) renderHTMLBlock(w *pdflib.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		m := bookmarkSentinelRe.FindStringSubmatch(string(seg.Value(source)))
		if m == nil {
			continue
		}
		idx, err := strconv.Atoi(m[1])
		if err != nil || idx < 0 || idx >= len(r.bookmarks) {
			continue
		}
		if bm, ok := w.Pdf.(interface {
			Bookmark(text string, level int, y float64)
		}); ok {
			entry := r.bookmarks[idx]
			bm.Bookmark(entry.Text, entry.Level, -1)
		}
	}
	return ast.WalkContinue, nil
}

// RenderPDF writes doc as a PDF: one section per turn, chart images
// embedded inline, and one /Outlines sidebar bookmark per turn so a reader
// can jump straight to any question or answer.
func RenderPDF(ctx context.Context, doc *ExportDoc, w io.Writer) error {
	md, bookmarks := ToMarkdown(doc)

	fpdf := &bookmarkFpdf{Fpdf: pdflib.NewFpdf(ctx, pdflib.FpdfConfig{
		Title:       doc.Conversation.Title,
		Subject:     "LiveReview Chat Export",
		Orientation: "P",
		PaperSize:   "A4",
	}, nil)}

	renderer := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			pdflib.New(
				pdflib.WithPDF(fpdf),
				pdflib.WithEscapeHTML(false),
				pdflib.WithNodeRenderers(
					util.Prioritized(&bookmarkNodeRenderer{bookmarks: bookmarks}, 100),
				),
			),
		),
	)

	return renderer.Convert([]byte(md), w)
}
