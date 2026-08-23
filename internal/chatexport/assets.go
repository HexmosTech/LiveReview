package chatexport

import _ "embed"

// logoIconPNG is the LiveReview eye mark, cropped square from
// assets/gfx/png/logo-with-text.png - used as the small running-header icon
// in the PDF and the masthead icon in the HTML export. The "LiveReview"
// wordmark itself is drawn as real text next to it in both renderers
// (rather than baked into the image) so it stays crisp at any size and
// matches each renderer's own font/color.
//
//go:embed assets/logo-icon.png
var logoIconPNG []byte

// Liberation Sans/Mono (SIL Open Font License - see assets/fonts/LICENSE.txt),
// embedded so the PDF renderer never depends on Google Fonts over the
// network (goldmark-pdf's own default fonts, e.g. Roboto, are fetched live
// on first use) or on fonts being installed on whatever machine runs the
// server. Liberation Sans is metrically compatible with Helvetica/Arial;
// unlike gofpdf's built-in core fonts, it's added as a real embedded UTF-8
// font, so typographic punctuation in both this document's own captions and
// in arbitrary LLM-authored message text (em dashes, curly quotes, ...)
// renders correctly instead of as mojibake.
var (
	//go:embed assets/fonts/LiberationSans-Regular.ttf
	fontSansRegular []byte
	//go:embed assets/fonts/LiberationSans-Bold.ttf
	fontSansBold []byte
	//go:embed assets/fonts/LiberationSans-Italic.ttf
	fontSansItalic []byte
	//go:embed assets/fonts/LiberationSans-BoldItalic.ttf
	fontSansBoldItalic []byte
	//go:embed assets/fonts/LiberationMono-Regular.ttf
	fontMonoRegular []byte
	//go:embed assets/fonts/LiberationMono-Bold.ttf
	fontMonoBold []byte
)

const (
	pdfSansFamily = "LRSans"
	pdfMonoFamily = "LRMono"
)
