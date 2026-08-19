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

A single question passes through up to four model calls, each a separate
conversation assembled from a different combination of this book's
chapters. The exact assembly — which chapters, and why — is its own
section: **Prompt Assembly**, immediately below. That section is the only
place this mapping is stated; nothing else in this book restates it.

Nothing is an ad-hoc prompt. Every instruction the model receives is a
numbered law in this book, and every answer it gives can cite the laws it
relied on.

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
