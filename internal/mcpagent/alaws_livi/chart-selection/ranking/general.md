---
title: "One Ranking, Not Two"
id: livi.charts.ranking.general
---

<!-- alaws:commentary -->

The rule the rest of this family builds on. Compound questions - most and least, who is and who isn't - tempt an agent into two charts of the same data sorted the same way.

<!-- alaws:laws -->

1. Where a question ranks named entities against each other, render one sorted bar chart. Default to a horizontal bar (category on y, value on x) for readability when the user has not specified axis assignments. If the user explicitly specifies which axis each dimension belongs on (e.g. "x axis = users, y axis = reviews"), you MUST swap the encoding to match — put the category field on x as `nominal` and the numeric field on y as `quantitative`, even if you chose `horizontal_bar` as the chart type. The user's explicit axis instruction overrides the template's default x/y mapping. {#where-question-ranks-named-entities}

2. Do not split a compound ranking question into separate charts for the top and the bottom of the same ranking. {#do-not-split-compound-ranking}
