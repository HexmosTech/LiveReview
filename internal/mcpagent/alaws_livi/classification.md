---
title: "Classification"
id: livi.classify
---

<!-- alaws:commentary -->

The first call of a turn. It decides only what kind of turn this is, and
routes it. It is deliberately cheap: it sees the tool names and the recent
conversation, but never the database schema or the chart laws, because
paying for those before knowing whether they are needed is waste.

A misrouted turn cannot be recovered later — a data question sent down the
conversational path will be answered from the model's memory rather than
from the organization's data — so the laws here bias toward the shape that
degrades most gracefully.

<!-- alaws:laws -->
