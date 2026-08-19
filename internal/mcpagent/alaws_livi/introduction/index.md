---
title: "Chapter and Section Index"
id: livi.intro.index
---

<!-- alaws:commentary -->

This section is a lookup table, not a law source — same as
[Prompt Assembly](#/books/internal%2Fmcpagent%2Falaws_livi/livi.intro.assembly)
above, it states no laws and is not sent to the model. Its job is to let a
person answer two questions without opening twenty files: *which chapter
covers this kind of question*, and *has a question like this actually been
asked before, and by which law was it answered*.

## Chart families

[Chart Selection](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts)
is organized by the comparison a question asks for, not by its topic —
see that chapter's own commentary for why. This is the routing table: find
the row that matches the question, go to that family.

| Family | Covers | Questions about |
|---|---|---|
| [5.2 Trend Over Time](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.trend) | a measure moving over time | adoption rising or falling, one repository's velocity over time, LOC against review count, whether reviews are getting faster |
| [5.3 Activity Rhythm](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.rhythm) | patterns across calendar days | daily habit, workflow rhythm, activity across repositories and days |
| [5.4 Distribution](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.distribution) | the shape of a spread | how broad adoption is, reviews per commit, who carries a repository |
| [5.5 Ranking](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.ranking) | who did the most or least | who uses it most and least, who needs a nudge |
| [5.6 Concentration](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.concentration) | whether a few dominate a total | whether a few repositories or people account for most of the total |
| [5.7 Period-Over-Period Comparison](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.comparison) | two points in time | gaining or losing velocity, what changed between two weeks, why velocity moved |
| [5.8 Relationship Between Measures](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.relationship) | two measures against each other | unusually active or inactive repositories, who gets the most value, review coverage |
| [5.9 Composition of a Total](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.composition) | the mix shifting over time | where reviews are triggered from, whether review is shifting earlier |
| [5.10 Two Categories Crossed](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.crosstab) | two categorical dimensions | severity against trigger, issues by repository and file — both currently unanswerable, see [Unavailable Data](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general.unavailable) |
| [5.11 Specialised Shapes](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff) | everything with no shared mechanism | adoption breadth over time, what engineers work on, cost savings, long-span LOC, feedback trust, issue categories, workflow health |

## Worked examples

Every real question that has driven a section in this book, and the law
that governs it. These are illustrations, not a menu — a question that
resembles one of these is governed by the same law even if the wording is
nothing alike, and a question resembling none of them is still answerable
under [General](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general)
alone.

| # | Question | Law |
|---|---|---|
| 1 | Is LiveReview adoption increasing since my team started using it? | [5.2.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.trend.counted_event) |
| 2 | Are engineers actually incorporating reviews into their daily workflow? | [5.3.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.rhythm.habit) |
| 3 | How broadly has the organization adopted LiveReview? | [5.4.4](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.distribution.spread) |
| 4 | Who has adopted LiveReview — and who hasn't? | [5.5.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.ranking.against_target) |
| 5 | Is adoption becoming broader over time? | [5.11.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.depth) |
| 6 | Which repositories are gaining or losing engineering velocity? | [5.7.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.comparison.direction) |
| 7 | Where is organizational velocity concentrated? | [5.6.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.concentration.entities) |
| 8 | Which repositories are unusually active or inactive? | [5.8.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.relationship.two_measures) |
| 9 | What does engineering activity look like across repositories and days? | [5.3.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.rhythm.entities) |
| 10 | What happened to a repository's velocity? | [5.2.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.trend.named_entity) |
| 11 | Why did this repository's velocity change? | [5.7.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.comparison.explaining) |
| 12 | Which engineers are carrying the repository? | [5.4.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.distribution.individuals) |
| 13 | What does each engineer actually spend their review activity on? | [5.11.4](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.member_composition) |
| 14 | Where are reviews happening? | [5.9.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.composition.shift) |
| 15 | Are we moving review earlier in the development lifecycle? | [5.9.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.composition.shift) |
| 16 | Are serious issues being caught before PR/MR? | [5.10.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.crosstab.quality_process) — blocked, see [Unavailable Data](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general.unavailable) |
| 17 | What kinds of engineering problems is LiveReview finding? | [5.11.5](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.missing_type) |
| 18 | Where are the issues concentrated? | [5.10.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.crosstab.quality_process) — blocked, see [Unavailable Data](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general.unavailable) |
| 19 | What is the blast radius of issues being caught? | [5.8.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.relationship.two_measures) — blocked, see [Unavailable Data](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general.unavailable) |
| 20 | How much does LiveReview save versus alternatives? | [5.11.6](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.net_figure) |
| 21 | How much code has LiveReview reviewed? | [5.11.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.long_span) |
| 22 | Are reviews getting faster? | [5.2.4](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.trend.spread) |
| 23 | How much engineering work is being covered by LiveReview? | [5.2.5](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.trend.two_measures) |
| 24 | Are reviews becoming more iterative? | [5.4.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.distribution.per_item) |
| 25 | Which engineers are getting the most value from LiveReview? | [5.8.3](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.relationship.two_measures) |
| 26 | Are people trusting the reviews? | [5.11.7](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.opposing) |
| 27 | Which repositories have the highest review coverage? | [5.8.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.relationship.ratio) |
| 28 | What does a healthy engineering-review workflow look like? | [5.11.8](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.oneoff.trajectory) — blocked, see [Unavailable Data](#/books/internal%2Fmcpagent%2Falaws_livi/livi.general.unavailable) |
| 29 | How much of the organization's activity is covered by the top users? | [5.6.2](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.concentration.entities) |
| 30 | What changed between week 1 and week 2? | [5.7.4](#/books/internal%2Fmcpagent%2Falaws_livi/livi.charts.comparison.metrics) |

<!-- alaws:laws -->
