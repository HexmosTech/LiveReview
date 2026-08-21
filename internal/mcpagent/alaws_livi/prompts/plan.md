---
title: "Plan Prompt"
id: livi.prompts.plan
---

<!-- alaws:commentary -->

Call #2. Assembled from General, the whole of Chart Selection, and
Planning.

**Known gap, not yet resolved:** the running server's actual plan prompt
(built in `laws.go`, not from this file) additionally strips every
worked-example Vega-Lite spec out of Chart Selection before sending it
here — Planning decides what to group by, never draws anything, so those
examples are pure token cost for this call (they cut the rendered prompt
from roughly 11.7k tokens to 7.6k). PromptBook's `{{ref:}}` only resolves
at whole-section granularity, and the spec examples live as individual
laws mixed into the same sections as the grouping rules Planning does
need — so this file cannot express that filter without listing every
non-spec law individually across all ~30 chart families, which is not
worth the fragility. Until PromptBook supports law-level exclusion (or
the specs move to their own sub-sections), the production plan prompt
stays hand-assembled in `laws.go`; this file is the unfiltered reference
version, useful for browsing the full composition but not identical to
what the server actually sends.

<!-- alaws:promptTemplate -->

{{ref:livi.general.principles}}

{{ref:livi.general.reading}}

{{ref:livi.general.data}}

{{ref:livi.general.precedence}}

{{ref:livi.general.unavailable}}

{{ref:livi.charts.basics}}

{{ref:livi.charts.trend.general}}

{{ref:livi.charts.trend.counted_event}}

{{ref:livi.charts.trend.named_entity}}

{{ref:livi.charts.trend.spread}}

{{ref:livi.charts.trend.two_measures}}

{{ref:livi.charts.rhythm.general}}

{{ref:livi.charts.rhythm.entities}}

{{ref:livi.charts.rhythm.habit}}

{{ref:livi.charts.distribution.general}}

{{ref:livi.charts.distribution.individuals}}

{{ref:livi.charts.distribution.per_item}}

{{ref:livi.charts.distribution.spread}}

{{ref:livi.charts.ranking.general}}

{{ref:livi.charts.ranking.against_target}}

{{ref:livi.charts.concentration.general}}

{{ref:livi.charts.concentration.entities}}

{{ref:livi.charts.comparison.general}}

{{ref:livi.charts.comparison.direction}}

{{ref:livi.charts.comparison.explaining}}

{{ref:livi.charts.comparison.metrics}}

{{ref:livi.charts.relationship.general}}

{{ref:livi.charts.relationship.ratio}}

{{ref:livi.charts.relationship.two_measures}}

{{ref:livi.charts.composition.general}}

{{ref:livi.charts.composition.shift}}

{{ref:livi.charts.composition.moment}}

{{ref:livi.charts.crosstab.general}}

{{ref:livi.charts.crosstab.quality_process}}

{{ref:livi.charts.oneoff.general}}

{{ref:livi.charts.oneoff.depth}}

{{ref:livi.charts.oneoff.long_span}}

{{ref:livi.charts.oneoff.member_composition}}

{{ref:livi.charts.oneoff.missing_type}}

{{ref:livi.charts.oneoff.net_figure}}

{{ref:livi.charts.oneoff.opposing}}

{{ref:livi.charts.oneoff.trajectory}}

{{ref:livi.planning.counting}}

{{ref:livi.planning.compound}}

{{ref:livi.planning.output}}
