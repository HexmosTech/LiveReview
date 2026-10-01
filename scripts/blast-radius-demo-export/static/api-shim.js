// Static-export fetch shim. Intercepts every API call the LiveReview
// review UI makes and serves embedded JSON captured from a real review, so
// this page works fully offline with zero backend/network dependency.
// Anything not explicitly matched below returns a harmless empty 200 so a
// stray action click (feedback, retract, handoff) never throws instead of
// silently failing -- it just won't do anything, which is correct for a
// read-only static demo.
(function () {
    const REVIEW_ID = "12737";
    const DATA_BASE = "./data/";

    const routes = [
        { test: (p) => p === "/api/review", file: "review.json" },
        { test: (p) => p === `/api/v1/diff-review/${REVIEW_ID}`, file: "diffreview.json" },
        { test: (p) => p === `/api/v1/diff-review/${REVIEW_ID}/events`, file: "events.json" },
        { test: (p) => p === "/api/blastradius", file: "blastradius.json" },
        { test: (p) => p === "/api/runtime/usage-chip", file: "usage-chip.json" },
    ];

    const cache = {};
    async function loadData(file) {
        if (!cache[file]) {
            cache[file] = fetch(DATA_BASE + file, { _passthrough: true }).then((r) => r.json());
        }
        return cache[file];
    }

    const realFetch = window.fetch.bind(window);
    window.fetch = async function (input, init) {
        if (init && init._passthrough) {
            const { _passthrough, ...rest } = init;
            return realFetch(input, rest);
        }
        const urlStr = typeof input === "string" ? input : input.url;
        let pathname;
        try {
            pathname = new URL(urlStr, window.location.origin).pathname;
        } catch (e) {
            pathname = urlStr;
        }

        for (const route of routes) {
            if (route.test(pathname)) {
                const data = await loadData(route.file);
                return new Response(JSON.stringify(data), {
                    status: 200,
                    headers: { "Content-Type": "application/json" },
                });
            }
        }

        // Unmatched call (draft/feedback/handoff/retract -- all
        // user-triggered action endpoints with no effect in a static
        // export). Respond harmlessly instead of hitting the network.
        console.warn("[static-export] no-op for unmatched fetch:", pathname);
        return new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } });
    };

    // PrecommitBar also opens an EventSource for /api/draft/events when
    // interactive; review.json ships with interactive:false so that path
    // never runs, but stub EventSource defensively anyway.
    window.EventSource = function () {
        return { close() {}, addEventListener() {}, removeEventListener() {} };
    };
})();
