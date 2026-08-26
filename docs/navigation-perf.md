 ▐▛███▛█   Claude Code v2.1.246
▝▜██████▀  Sonnet 5 · Claude Pro
  ▝▝ ▝▝    ~/bin/LiveReview


❯ One of the main problems in LiveReview UI now is -- on click of any link, it  
  takes a while for the next page to load up or something. And there's no       
  indication, no immediate visual feedback and the next page is a bit slow to   
  load. This is not the "first load" problem, but more like navigation once the 
  app is there. Come up with an RCA and options to make the app way more        
  responsive /plan                                                              

● I'll investigate the frontend navigation/routing setup to build an
  evidence-based RCA before proposing options.

  Searched for 5 patterns, read 1 file (ctrl+o to expand)

● There's already a perf doc referenced in the code. Let me check it and the
  Navbar link handling plus webpack chunking config.

  Searched for 9 patterns, read 2 files, ran 1 shell command (ctrl+o to expand)

● Entered plan mode
  Claude is now exploring and designing an implementation approach.

  Searched for 1 pattern, read 1 file (ctrl+o to expand)

● Updated plan
  ⎿  /plan to preview

● User approved Claude's plan
  ⎿  Plan saved to: ~/.claude/plans/fuzzy-kindling-plum.md · /plan to edit
     Fix sluggish in-app navigation (post-load, not cold start)

     Context

     Users report that clicking a link inside LiveReview (already-loaded app,
     not first paint) has a
     noticeable delay before anything visible happens, and there's no feedback
     that the click
     registered. This is separate from the cold-load perf work already done (see
     the
     splitChunks/vendor-* comments in ui/webpack.config.js and the
     staleTime/shared-cache
     comment in ui/src/api/queryClient.ts, both referencing a prior
     "docs/perf-improvement.md"
     effort — that file no longer exists in the repo, but its fixes are still in
     place).

     RCA (confirmed by reading the code, not guessing)

     1. Every route is React.lazy, with zero prefetching (ui/src/App.tsx:16-38).
        Nothing
        ever calls import() for a route ahead of time — not on link hover, not
        during browser idle
        time after the shell mounts. So the first time a user clicks into any
        given route this
        session, the click itself is what triggers the network fetch +
        parse/eval of that route's JS
        chunk(s), and the user waits on the critical path for exactly the part
        of the work that
        could have happened earlier, invisibly.
     2. No feedback between click and content. Navbar.tsx:652 (handleNavClick)
        just calls
        onNavigate and closes menus — no pressed/loading affordance on the link
        itself. The only
        loading UI in the app is RouteFallback (App.tsx:106), a full-screen
        spinner
        (FullScreenLoader, App.tsx:97-104) wrapped around the entire <Routes>
        block
        (App.tsx:298-330). Two consequences:
        - Between the click and React actually suspending, there is no visual
          change at all — the
          click feels ignored.
        - Once it does appear, it wipes the whole page (nav/shell included
          visually feels frozen,
          but the content area blanks to a centered spinner) rather than a
          lightweight in-place
          transition — jarring even for a fast, already-cached navigation.
     3. Data fetching is not the primary offender. ui/src/api/queryClient.ts
        already runs a
        shared TanStack Query cache with staleTime: 30_000, so revisiting a
        route within 30s
        reuses cached data instead of refetching. The bottleneck is specifically
        the JS chunk
        fetch/parse/eval on first visit to a route each session, compounded by
        zero visual feedback.
     4. No instrumentation. There are no perf marks around click → chunk-loaded
        → paint, so this
        regressed/persisted invisibly and there's no way to verify a fix
        quantitatively short of
        manually watching the Network tab.

     Approach

     Two independent, additive changes — do both, they fix different halves of
     the complaint:

     1. Prefetch route chunks before the click (removes the wait)

     Add ui/src/utils/routePrefetch.ts: a small map from route key → the same
     import() used in
     each React.lazy(() => import(...)) call in App.tsx, wrapped in a per-key
     in-flight/done
     cache so calling prefetch twice is a no-op. Reuse the exact same dynamic
     import expressions
     already in App.tsx:16-38 (don't introduce a second import path for the same
     module, which
     would risk webpack emitting a duplicate chunk).

     Wire two triggers:
     - Hover/focus intent: in Navbar.tsx (nav links, ~L822-828) and
       NavMegaMenu.tsx, add
       onMouseEnter/onFocus handlers that call the prefetch for that link's
       target route. This is
       the standard "quicklink" pattern — by the time the click lands, the chunk
       request is already
       in flight or done.
     - Idle-time warmup: in App.tsx, after the authenticated shell mounts, use
       requestIdleCallback (with a setTimeout fallback) to prefetch a short list
       of the
       most-likely-next routes (Dashboard, Reviews, Settings) once, so even a
       direct click with no
       prior hover (e.g. keyboard nav, touch) is often already warm.

     2. Immediate visual feedback + lighter in-flight transition

     - Add ui/src/components/NavigationProgressBar.tsx: a slim top-of-viewport
       bar (indeterminate
       progress or width-animated) that starts as soon as a navigation is
       triggered and completes
       when the new route has rendered. Drive it off useLocation() changes plus
       the click handlers
       already centralized in Navbar.handleNavClick/App.handleNavigate — start
       on click, stop in
       a useEffect keyed on location.pathname (mirrors the existing activePage
       effect at
       App.tsx:168-177). Mount it once in AppContent alongside the existing
       Toaster.
     - Give the clicked nav link an immediate pressed/active visual (a transient
       class toggled in
       handleNavClick) so the click has a visible effect in the same frame,
       before any network
       activity resolves.
     - Do not remove RouteFallback/Suspense (still needed for genuinely cold
       chunks), but
       since (1) makes most navigations already-warm, it will rarely be seen
       full-screen going
       forward — leave it as the fallback-of-last-resort.

     Files touched

     - ui/src/utils/routePrefetch.ts (new)
     - ui/src/components/NavigationProgressBar.tsx (new)
     - ui/src/App.tsx — mount progress bar, idle-time prefetch warmup, pass
       prefetch trigger into handleNavigate
     - ui/src/components/Navbar/Navbar.tsx — hover/focus prefetch + pressed
       state on nav links
     - ui/src/components/Navbar/NavMegaMenu.tsx — same, for mega-menu links

     Out of scope (noted, not doing now)

     - Perf-mark/RUM instrumentation — worth adding later but not required to
       fix the reported
       symptom; can be a fast follow once the above is in and we want to
       quantify the win.
     - Further webpack chunk-splitting changes — current splitChunks config is
       deliberately tuned
       (see the detailed comments in webpack.config.js:264-346); no evidence
       chunk size is the
       problem here, only when the fetch starts.

     Verification

     - cd ui && npx tsc --noEmit for type safety (per project convention: never
       run npm run build
       without asking first).
     - Run the dev server (npm run start in ui/), open DevTools Network tab, and
       confirm:
       - Hovering a nav link fires the chunk request before the click.
       - Clicking a link shows the progress bar / pressed state on the same
         frame as the click.
       - Navigating to an already-hovered/prefetched route no longer shows the
         full-screen
         RouteFallback spinner.
     - Manually click through Dashboard → Reviews → Settings → Explore → Chat
       and back, confirming
       no regression in auth-gated redirects (App.tsx:180-186) or the /chat
       special-casing
       (Footer/LicenseStatusBar hidden on /chat, App.tsx:295/333/335).

● Plan approved. Starting implementation now: prefetch utility, hover/idle
  prefetching wired into the nav, and a navigation progress bar for immediate
  click feedback.

  Read 1 file, ran 1 shell command (ctrl+o to expand)
  ⎿  Interrupted · What should Claude do instead?

❯ /export docs/navigation-perf.md                                               
  ⎿  Conversation exported to:
     /home/shrsv/bin/LiveReview/docs/navigation-perf.md