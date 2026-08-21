---
title: "Interpreting"
id: livi.interpreting
---

<!-- alaws:commentary -->

A single call that replaces the old Planning + Finalizing two-step. The model
receives the user's question together with the live dbctx schema context, and
returns up to five interpretations — each a self-contained SQL query plus the
chart type and encoding that best presents its result.

This is where answers are most often lost. A question like "how has our review
activity changed?" can be read as reviews-per-month (trend), reviews-per-
contributor (ranking), reviews-per-trigger-type (composition), or reviews-per-
repository (distribution). The old pipeline forced one reading; this one
produces several and lets the data decide which are worth showing.

<!-- alaws:laws -->
