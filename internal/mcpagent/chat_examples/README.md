# chat_examples

Sample Vega-Lite chart payloads matching Livi's chat response format
(`title` / `description` / `query` / `spec`), with real sample data
embedded so they render standalone.

## Files

- `01_bar_chart.json` — reviews per month
- `02_line_chart.json` — 14-day daily trend
- `03_scatter_chart.json` — reviews vs. LOC per engineer
- `04_grouped_bar_chart.json` — reviews per engineer by month
- `05_pie_chart.json` — status breakdown

## Render to PNG

Requires [vl-convert](https://github.com/vega/vl-convert):

```bash
cargo install vl-convert --locked
```

Then:

```bash
./render.sh
```

This renders every `*.json` in this directory to a same-named `.png`
right here (`01_bar_chart.json` -> `01_bar_chart.png`, etc).

## Render one file manually

```bash
jq '.spec' 01_bar_chart.json > /tmp/spec.json
vl-convert vl2png --input /tmp/spec.json --output 01_bar_chart.png
```

(`vl-convert` only accepts a raw Vega-Lite spec, not the outer
`title`/`description`/`spec` wrapper Livi uses, so `.spec` has to be
pulled out first.)

## Render without installing anything

Paste the `spec` object from any file into the
[Vega-Lite Editor](https://vega.github.io/editor/).
