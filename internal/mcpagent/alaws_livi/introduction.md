---
title: "Introduction"
id: livi.intro
---

<!-- alaws:commentary -->

This book governs Livi, LiveReview's analytics assistant. An engineering
leader asks how their team is using LiveReview; Livi answers with a chart
drawn from that organization's own data.

**This chapter states no laws.** It exists so that someone who has never
seen the code can read the book end to end and understand how the
assistant is governed. It is not sent to the model.

## How a question is answered

A single question passes through up to four model calls. Each call is a
separate conversation with its own instructions, and each one is assembled
from the chapters of this book:

**Classification** — is this a data question, an action, or conversation?
*Sent: General + Classification.*

**Planning** — what to count, and along which dimension.
*Sent: General + Planning + Chart Selection.*

**Finalizing** — the query that produces the answer, and the shape it takes.
*Sent: General + Chart Selection + Finalizing.*

**Degraded paths** — a rejected query, or a result with no rows.
*Sent: General + Degraded Paths.*

Nothing is an ad-hoc prompt. Every instruction the model receives is a
numbered law in this book, and every answer it gives can cite the laws it
relied on.

## Why the chapters are split this way

The chapters are not subject areas — they are the stages of the pipeline,
because that is what determines which laws a given call needs to see.

**General** is the exception: it is sent with every call. It holds the
obligations that hold regardless of what is being asked — never state a
figure you cannot point at, scope every query to the organization, ask
when the subject is ambiguous.

**Chart Selection** is the other exception: it is sent to two stages. The
choice of chart is settled at Finalizing, but the grouping that makes that
chart possible is settled at Planning — a rhythm question has to be
grouped by day before anything can draw a calendar. Sending these laws
only to Finalizing produces a correct chart of the wrong data, which is
the most common failure this book was written to prevent.

## How to read a section

Each section states the situation it governs, then the obligations that
follow, numbered so they can be cited. Commentary — the prose above the
laws — carries the reasoning, worked examples and chart specifications.
Commentary is for people; only the numbered laws are sent to the model.

## What is not here

Livi's conversational replies and its tool-calling actions — triggering a
review, creating a learning — are governed elsewhere. This book covers the
analytics path only.

<!-- alaws:laws -->
