package chatexport

import (
	"bytes"
	"context"
	"image/color"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/go-swiss/fonts"
	"github.com/phpdave11/gofpdf"
	pdflib "github.com/stephenafamo/goldmark-pdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/util"
)

// bookmarkSentinelRe matches the <!--export-bookmark:N--> sentinel comment
// ToMarkdown emits immediately before each heading.
var bookmarkSentinelRe = regexp.MustCompile(`<!--export-bookmark:(\d+)-->`)

// Brand palette, shared by the PDF and HTML renderers so a reader sees the
// same document whichever format they open. Alpha is always 255: goldmark-pdf's
// SetStyle extracts RGB via color.Color.RGBA(), which is alpha-premultiplied -
// a color with A:0 collapses to black no matter its R/G/B, so every color
// used as a Style.TextColor/FillColor here must carry a real alpha or it
// silently renders wrong.
var (
	colBrand   = rgba(37, 99, 235)   // blue-600 - title, links, accents
	colNavy    = rgba(15, 23, 42)    // slate-900 - body/heading text
	colMuted   = rgba(100, 116, 139) // slate-500 - captions, meta text
	colBorder  = rgba(226, 232, 240) // slate-200 - rules, table borders
	colTableBg = rgba(248, 250, 252) // slate-50 - table header fill
	colWhite   = rgba(255, 255, 255)
	colQuoteFg = rgba(30, 64, 175) // blue-800 - user (quoted) turn text
)

func rgba(r, g, b uint8) color.Color { return color.RGBA{R: r, G: g, B: b, A: 255} }

// rgbInts converts a palette color to the plain (0-255) ints gofpdf's raw
// SetTextColor/SetDrawColor/SetFillColor take - used for header/footer/rule
// chrome, which draws directly against *gofpdf.Fpdf rather than through
// goldmark-pdf's Style system (see drawRunningHeader).
func rgbInts(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

const (
	pdfMarginSide       = 46.0
	pdfMarginTopContent = 84.0 // reserves room for the running header banner
	pdfBottomBreak      = 58.0 // reserves room for the footer
	pdfLogoIconName     = "livereview-icon"
	pdfLogoIconSize     = 15.0
)

// pdfSans and pdfMono are goldmark-pdf Font values for the embedded
// Liberation families (see assets.go) rather than one of the library's own
// FontHelvetica/FontCourier/Font* constants: Type: FontTypeCustom tells
// addStyleFonts to skip its normal Google Fonts network-loading path
// entirely (see fonts.go's AddFonts - it only continues past inbuilt/custom
// types), since registerFonts already embeds the real bytes directly into
// the *gofpdf.Fpdf before any content renders.
var (
	pdfSans = pdflib.Font{CanUseForText: true, Family: pdfSansFamily, Type: pdflib.FontTypeCustom}
	pdfMono = pdflib.Font{CanUseForCode: true, Family: pdfMonoFamily, Type: pdflib.FontTypeCustom}
)

// registerFonts embeds the Liberation Sans/Mono TTFs (see assets.go) into
// raw under the family names exportStyles references. gofpdf's
// AddUTF8FontFromBytes is idempotent (keyed by family+style, a no-op past
// the first call), so this is safe and cheap to call defensively wherever
// text might be drawn with these families - including from the header/
// footer closures, which can fire (via gofpdf's HeaderFunc/FooterFunc) at
// points during page construction that run before RenderPDF's own setup
// code gets a chance to register anything.
func registerFonts(raw *gofpdf.Fpdf) {
	raw.AddUTF8FontFromBytes(pdfSansFamily, "", fontSansRegular)
	raw.AddUTF8FontFromBytes(pdfSansFamily, "B", fontSansBold)
	raw.AddUTF8FontFromBytes(pdfSansFamily, "I", fontSansItalic)
	raw.AddUTF8FontFromBytes(pdfSansFamily, "BI", fontSansBoldItalic)
	raw.AddUTF8FontFromBytes(pdfMonoFamily, "", fontMonoRegular)
	raw.AddUTF8FontFromBytes(pdfMonoFamily, "B", fontMonoBold)
}

// exportStyles is the document's full type/color system, replacing
// goldmark-pdf's generic defaults so the PDF and HTML exports read as one
// consistent, deliberately-designed document rather than two different
// renderings of the same data. Heading levels are repurposed by role, not
// by nesting depth: H1 is the document title (cover only), H3 is the
// subtitle (visibly smaller/lighter than H1 - previously H2 was used for
// both and the two were hard to tell apart), H2 is a conversation section
// heading in a multi-conversation compile, and H6 is a small "kicker"
// caption reused for every piece of de-emphasized meta text (generated-on
// line, conversation meta, and the per-turn role/time label) so repetitive
// information never competes visually with the actual content.
func exportStyles() pdflib.Styles {
	s := pdflib.Styles{}

	s.H1 = &pdflib.Style{Font: pdfSans, Size: 27, Spacing: 8, TextColor: colBrand, FillColor: colWhite}
	s.H2 = &pdflib.Style{Font: pdfSans, Size: 17, Spacing: 6, TextColor: colNavy, FillColor: colWhite}
	s.H3 = &pdflib.Style{Font: pdfSans, Size: 13, Spacing: 5, TextColor: colMuted, FillColor: colWhite}
	s.H4 = &pdflib.Style{Font: pdfSans, Size: 13, Spacing: 5, TextColor: colNavy, FillColor: colWhite}
	s.H5 = &pdflib.Style{Font: pdfSans, Size: 12, Spacing: 4, TextColor: colNavy, FillColor: colWhite}
	s.H6 = &pdflib.Style{Font: pdfSans, Size: 9, Spacing: 10, TextColor: colMuted, FillColor: colWhite}

	// Spacing also sets the gap goldmark-pdf puts *between list items*
	// (there's no separate List style in this library - list rendering
	// reuses Normal's line height for that gap too), which was reading as
	// far too airy for the chart stat bullets (see markdown.go's per-stat
	// list items). 3 keeps body-text line height comfortable (14pt on an
	// 11pt font, ~1.27x) while meaningfully tightening list spacing.
	s.Normal = &pdflib.Style{Font: pdfSans, Size: 11, Spacing: 3, TextColor: colNavy, FillColor: colWhite}
	s.Blockquote = &pdflib.Style{Font: pdfSans, Size: 11, Spacing: 5, TextColor: colQuoteFg, FillColor: colWhite}

	s.THeader = &pdflib.Style{Font: pdfSans, Size: 9.5, Spacing: 4, TextColor: colMuted, FillColor: colTableBg}
	s.TBody = &pdflib.Style{Font: pdfSans, Size: 9.5, Spacing: 4, TextColor: colNavy, FillColor: colWhite}

	s.CodeFont = pdfMono
	s.CodeBlockTheme = styles.Get("github")
	s.LinkColor = colBrand

	return s
}

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

// exportNodeRenderer overrides two node kinds goldmark-pdf's own renderer
// handles unsuitably for this document:
//
//   - ast.KindHTMLBlock: goldmark-pdf's default renderer for this kind is a
//     no-op ("Cannot process HTML blocks"), so the <!--export-bookmark:N-->
//     and <!--export-pagebreak--> sentinel comments ToMarkdown emits would
//     otherwise be silently dropped instead of registering a PDF /Outlines
//     entry or starting a new page.
//   - ast.KindThematicBreak: the library's default "---" rule is a thick
//     (3pt) flat gray line that doesn't match this document's palette; this
//     draws a thin rule in the same border color used everywhere else.
type exportNodeRenderer struct {
	bookmarks []BookmarkEntry
}

func (r *exportNodeRenderer) RegisterFuncs(reg pdflib.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
}

func (r *exportNodeRenderer) renderHTMLBlock(w *pdflib.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		line := string(seg.Value(source))

		if strings.TrimSpace(line) == strings.TrimSpace(pageBreakSentinel) {
			w.Pdf.AddPage()
			continue
		}

		m := bookmarkSentinelRe.FindStringSubmatch(line)
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

func (r *exportNodeRenderer) renderThematicBreak(w *pdflib.Writer, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	w.Pdf.BR(6)
	x := w.Pdf.GetX()
	y := w.Pdf.GetY()
	_, _, rm, _ := w.Pdf.GetMargins()
	width, _ := w.Pdf.GetPageSize()
	br, bg, bb := rgbInts(colBorder)
	w.Pdf.SetLineWidth(0.75)
	w.Pdf.SetFillColor(uint8(br), uint8(bg), uint8(bb))
	w.Pdf.Line(x, y, width-rm, y)
	w.Pdf.BR(10)
	return ast.WalkContinue, nil
}

// registerLogoIcon registers the embedded LiveReview mark under a fixed
// name, idempotently: gofpdf caches image registrations by name and
// RegisterImageOptionsReader is a no-op once that name is present, so it's
// safe (and necessary, since drawing can happen before RenderPDF has a
// chance to register anything - the header fires during page 1's
// construction) to call this right before every draw.
func registerLogoIcon(raw *gofpdf.Fpdf) {
	raw.RegisterImageOptionsReader(pdfLogoIconName, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, bytes.NewReader(logoIconPNG))
}

// drawCoverMark draws the small icon + "LiveReview" wordmark once, at the
// very top of the document (page 1 only - drawRunningHeader handles every
// later page), then leaves the cursor positioned below it for the title
// that goldmark is about to render.
func drawCoverMark(raw *gofpdf.Fpdf) {
	registerLogoIcon(raw)
	registerFonts(raw)
	x, y := pdfMarginSide, raw.GetY()

	raw.ImageOptions(pdfLogoIconName, x, y, pdfLogoIconSize+4, pdfLogoIconSize+4, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, 0, "")
	raw.SetFont(pdfSansFamily, "B", 12)
	raw.SetTextColor(rgbInts(colBrand))
	raw.SetXY(x+pdfLogoIconSize+14, y+2)
	raw.CellFormat(200, pdfLogoIconSize+4, "LiveReview", "", 0, "L", false, 0, "")

	raw.SetXY(x, y+pdfLogoIconSize+22)
}

// drawRunningHeader draws the compact running banner (icon, wordmark, and
// the document title, truncated if long) plus a thin rule, on every page
// after the cover. Chrome text (here, in drawCoverMark, and in drawFooter)
// uses the same embedded pdfSansFamily as the body copy (see registerFonts)
// rather than a gofpdf core font, so the document reads as one consistent
// typeface and so punctuation like the header's own truncation ellipsis
// renders correctly.
func drawRunningHeader(raw *gofpdf.Fpdf, docTitle string) {
	if raw.PageNo() <= 1 {
		return
	}
	registerLogoIcon(raw)
	registerFonts(raw)

	top := 26.0
	iconSize := pdfLogoIconSize
	raw.ImageOptions(pdfLogoIconName, pdfMarginSide, top, iconSize, iconSize, false, gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}, 0, "")

	raw.SetFont(pdfSansFamily, "B", 9.5)
	raw.SetTextColor(rgbInts(colBrand))
	raw.SetXY(pdfMarginSide+iconSize+8, top+2)
	raw.CellFormat(90, iconSize-4, "LiveReview", "", 0, "L", false, 0, "")

	title := truncateForHeader(docTitle, 60)
	raw.SetFont(pdfSansFamily, "", 9)
	raw.SetTextColor(rgbInts(colMuted))
	width, _ := raw.GetPageSize()
	_, _, rm, _ := raw.GetMargins()
	raw.SetXY(pdfMarginSide, top+2)
	raw.CellFormat(width-rm-pdfMarginSide, iconSize-4, title, "", 0, "R", false, 0, "")

	ruleY := top + iconSize + 6
	raw.SetLineWidth(0.75)
	raw.SetDrawColor(rgbInts(colBorder))
	raw.Line(pdfMarginSide, ruleY, width-rm, ruleY)

	// gofpdf's beginpage() sets y = tMargin before invoking the header
	// callback, but every draw call above (CellFormat, Line, ...) moves the
	// cursor to wherever it last drew - left alone, the body content that
	// goldmark renders next would start from there and overlap this banner
	// instead of from the reserved pdfMarginTopContent band.
	raw.SetXY(pdfMarginSide, pdfMarginTopContent)
}

func drawFooter(raw *gofpdf.Fpdf) {
	registerFonts(raw)
	width, height := raw.GetPageSize()
	_, _, rm, _ := raw.GetMargins()
	y := height - 40

	raw.SetLineWidth(0.75)
	raw.SetDrawColor(rgbInts(colBorder))
	raw.Line(pdfMarginSide, y, width-rm, y)

	raw.SetFont(pdfSansFamily, "", 8)
	raw.SetTextColor(rgbInts(colMuted))
	raw.SetXY(pdfMarginSide, y+7)
	raw.CellFormat(200, 12, "Generated by LiveReview", "", 0, "L", false, 0, "")

	raw.SetXY(pdfMarginSide, y+7)
	raw.CellFormat(width-rm-pdfMarginSide, 12, "Page "+strconv.Itoa(raw.PageNo())+" of {nb}", "", 0, "R", false, 0, "")
}

func truncateForHeader(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// RenderPDF writes doc as a PDF: a branded cover with the document title
// and subtitle, one section per conversation (starting on its own page
// when there's more than one), one per turn within it, chart images
// embedded inline, a running header/footer on every page after the cover,
// and a two-level /Outlines sidebar (conversation, then turn) so a reader
// can jump straight to any question or answer.
func RenderPDF(ctx context.Context, doc *CompiledDoc, w io.Writer) error {
	md, bookmarks := ToMarkdown(doc)

	fpdf := &bookmarkFpdf{Fpdf: pdflib.NewFpdf(ctx, pdflib.FpdfConfig{
		Title:       doc.Title,
		Subject:     "LiveReview Chat Export",
		Orientation: "P",
		PaperSize:   "A4",
		HeaderFunc: func(impl pdflib.Fpdf, _ fonts.Cache) func() {
			return func() { drawRunningHeader(impl.Fpdf, doc.Title) }
		},
		FooterFunc: func(impl pdflib.Fpdf, _ fonts.Cache) func() {
			return func() { drawFooter(impl.Fpdf) }
		},
	}, nil)}

	raw := fpdf.Fpdf.Fpdf
	raw.AliasNbPages("")
	raw.SetMargins(pdfMarginSide, pdfMarginTopContent, pdfMarginSide)
	raw.SetAutoPageBreak(true, pdfBottomBreak)
	raw.SetXY(pdfMarginSide, 40)
	drawCoverMark(raw)

	renderer := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRenderer(
			pdflib.New(
				pdflib.WithPDF(fpdf),
				pdflib.WithEscapeHTML(false),
				pdflib.OptionFunc(func(c *pdflib.Config) { c.Styles = exportStyles() }),
				pdflib.WithNodeRenderers(
					util.Prioritized(&exportNodeRenderer{bookmarks: bookmarks}, 100),
				),
			),
		),
	)

	return renderer.Convert([]byte(md), w)
}
