package crossrepolink

import (
	"testing"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/serverscore"
)

func TestMatchCalls_VersionAlias(t *testing.T) {
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes: []serverscore.RouteNode{
			{QualifiedName: "__route__POST__/sms/send-otp", Name: "/sms/send-otp", Method: "POST", FilePath: "src/sms.controller.ts"},
			{QualifiedName: "__route__POST__/unrelated", Name: "/unrelated", FilePath: "src/other.controller.ts"},
		},
	}
	graphs := map[string]*serverscore.RepoGraph{"target": target}
	calls := map[string][]Call{"main.sym": {{Method: "POST", Path: "/v1/sms/send-otp"}}}
	got := MatchCalls(calls, graphs, map[string]RepoMeta{"target": {ID: 3, FullName: "org/target"}}, true)
	impacts := got["main.sym"]
	if len(impacts) != 1 {
		t.Fatalf("MatchCalls = %+v, want one version-alias match", impacts)
	}
	if impacts[0].RelatedRepo != "target" || impacts[0].CallSite != "__route__POST__/sms/send-otp" {
		t.Fatalf("unexpected impact: %+v", impacts[0])
	}
	if impacts[0].MatchKind != "version-alias" || impacts[0].RelatedRepoName != "org/target" {
		t.Fatalf("match kind/enrichment = %+v", impacts[0])
	}
}

func TestMatchCalls_ExactAndAliasAreDistinguished(t *testing.T) {
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes: []serverscore.RouteNode{
			{QualifiedName: "__route__GET__/v1/orders", Name: "/v1/orders", Method: "GET", FilePath: "src/orders.ts"},
		},
	}
	graphs := map[string]*serverscore.RepoGraph{"target": target}
	calls := map[string][]Call{"sym": {{Method: "GET", Path: "/v1/orders"}}}
	got := MatchCalls(calls, graphs, nil, true)
	if len(got["sym"]) != 1 || got["sym"][0].MatchKind != "exact" {
		t.Fatalf("same versioning should be exact, got %+v", got["sym"])
	}
	// The worker's alias-only mode must not re-add exact matches that the
	// engine's CROSS_* edges already provided.
	aliasOnly := MatchCalls(calls, graphs, nil, false)
	if len(aliasOnly["sym"]) != 0 {
		t.Fatalf("alias-only mode returned exact match: %+v", aliasOnly["sym"])
	}
}

func TestMatchCalls_MethodMismatchDoesNotMatch(t *testing.T) {
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes: []serverscore.RouteNode{
			{QualifiedName: "__route__GET__/orders", Name: "/orders", Method: "GET", FilePath: "src/orders.ts"},
		},
	}
	graphs := map[string]*serverscore.RepoGraph{"target": target}
	calls := map[string][]Call{"sym": {{Method: "POST", Path: "/orders"}}}
	if got := MatchCalls(calls, graphs, nil, true); len(got["sym"]) != 0 {
		t.Fatalf("GET route matched a POST call: %+v", got["sym"])
	}
}

func TestMatchCalls_DedupesAcrossRepeatedCalls(t *testing.T) {
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes: []serverscore.RouteNode{
			{QualifiedName: "__route__POST__/orders", Name: "/orders", FilePath: "src/orders.ts"},
		},
	}
	graphs := map[string]*serverscore.RepoGraph{"target": target}
	calls := map[string][]Call{"sym": {
		{Method: "POST", Path: "/orders"},
		{Method: "POST", Path: "https://svc/orders?x=1"},
	}}
	got := MatchCalls(calls, graphs, nil, true)
	if len(got["sym"]) != 1 {
		t.Fatalf("duplicate call sites must produce one impact, got %+v", got["sym"])
	}
}

func TestBuildChains_WalksTwoHops(t *testing.T) {
	main := &serverscore.RepoGraph{Project: "main"}
	b := &serverscore.RepoGraph{
		Project:  "svc-b",
		Handlers: map[string][]string{"__route__POST__/orders": {"svc-b.OrdersController.create"}},
		Outbound: map[string][]serverscore.OutboundRoute{
			"svc-b.OrdersController.create": {{Name: "http://svc-c:8080/v1/ledger/entry"}},
		},
	}
	c := &serverscore.RepoGraph{
		Project:  "svc-c",
		Routes:   []serverscore.RouteNode{{QualifiedName: "__route__POST__/ledger/entry", Name: "/ledger/entry", FilePath: "src/ledger.controller.ts"}},
		Handlers: map[string][]string{"__route__POST__/ledger/entry": {"svc-c.LedgerController.create"}},
	}
	meta := map[string]RepoMeta{
		"svc-b": {ID: 2, FullName: "org/b", WebURL: "https://git/org/b"},
		"svc-c": {ID: 3, FullName: "org/c", WebURL: "https://git/org/c"},
	}
	impacts := []blastradius.CrossRepoImpact{{
		RelatedRepo: "svc-b", RelatedRepoID: 2, RelatedRepoName: "org/b",
		CallSite: "__route__POST__/orders", EdgeType: "CROSS_HTTP_CALLS",
	}}
	chains := BuildChains(main.Project, map[string]*serverscore.RepoGraph{"main": main, "svc-b": b, "svc-c": c}, meta, impacts, 3)
	if len(chains) != 1 {
		t.Fatalf("BuildChains = %+v, want one chain", chains)
	}
	hops := chains[0].Hops
	if len(hops) != 2 || hops[0].RelatedRepo != "svc-b" || hops[1].RelatedRepo != "svc-c" {
		t.Fatalf("chain order = %+v", hops)
	}
	if hops[1].RelatedRepoName != "org/c" || hops[1].CallSite != "__route__POST__/ledger/entry" {
		t.Fatalf("second hop = %+v", hops[1])
	}
}

func TestRelatedRepoGraph_HandlesCycleAndCap(t *testing.T) {
	adj := map[int64][]int64{
		1: {2},
		2: {1, 3},
		3: {2, 1},
	}
	got := RelatedRepoGraph(adj, 1, 3, 10)
	if len(got) != 2 {
		t.Fatalf("RelatedRepoGraph = %+v, want repos 2 and 3 once each", got)
	}
	if got[0].ID != 2 || got[0].Depth != 1 || got[1].ID != 3 || got[1].Depth != 2 {
		t.Fatalf("RelatedRepoGraph order/depth = %+v", got)
	}

	wide := map[int64][]int64{1: {2, 3, 4, 5}}
	if capped := RelatedRepoGraph(wide, 1, 3, 2); len(capped) != 2 {
		t.Fatalf("RelatedRepoGraph cap = %+v, want 2", capped)
	}
}

func TestBuildChains_CycleDoesNotLoop(t *testing.T) {
	main := &serverscore.RepoGraph{
		Project: "main",
		Routes:  []serverscore.RouteNode{{QualifiedName: "__route__POST__/main-callback", Name: "/main-callback", FilePath: "src/main.ts"}},
	}
	b := &serverscore.RepoGraph{
		Project:  "svc-b",
		Handlers: map[string][]string{"__route__POST__/orders": {"svc-b.handler"}},
		Outbound: map[string][]serverscore.OutboundRoute{
			"svc-b.handler": {
				{Name: "/main-callback"},
				{Name: "/v1/ledger"},
			},
		},
	}
	c := &serverscore.RepoGraph{
		Project:  "svc-c",
		Routes:   []serverscore.RouteNode{{QualifiedName: "__route__POST__/ledger", Name: "/ledger", FilePath: "src/ledger.ts"}},
		Handlers: map[string][]string{"__route__POST__/ledger": {"svc-c.handler"}},
		Outbound: map[string][]serverscore.OutboundRoute{
			"svc-c.handler": {{Name: "/orders"}},
		},
	}
	graphs := map[string]*serverscore.RepoGraph{"main": main, "svc-b": b, "svc-c": c}
	meta := map[string]RepoMeta{"svc-b": {ID: 2, FullName: "org/b"}, "svc-c": {ID: 3, FullName: "org/c"}}
	impacts := []blastradius.CrossRepoImpact{{RelatedRepo: "svc-b", RelatedRepoID: 2, RelatedRepoName: "org/b", CallSite: "__route__POST__/orders", EdgeType: "CROSS_HTTP_CALLS"}}
	chains := BuildChains(main.Project, graphs, meta, impacts, 3)
	if len(chains) != 1 {
		t.Fatalf("BuildChains with cycles = %+v, want exactly one [svc-b, svc-c] chain", chains)
	}
	if len(chains[0].Hops) != 2 || chains[0].Hops[0].RelatedRepo != "svc-b" || chains[0].Hops[1].RelatedRepo != "svc-c" {
		t.Fatalf("cycle chain = %+v", chains[0].Hops)
	}
}

func TestNormalizeRemoteRepoURL(t *testing.T) {
	cases := []struct{ in, host, path string }{
		{"https://git.apps.hexmos.com/framework-examples/typescript/nestjs/blive-microservice-2-consumer", "git.apps.hexmos.com", "framework-examples/typescript/nestjs/blive-microservice-2-consumer"},
		{"https://git.apps.hexmos.com/group/repo.git", "git.apps.hexmos.com", "group/repo"},
		{"ssh://git@git.apps.hexmos.com/group/repo.git", "git.apps.hexmos.com", "group/repo"},
		{"git@git.apps.hexmos.com:group/repo.git", "git.apps.hexmos.com", "group/repo"},
		{"https://git.apps.hexmos.com/group/repo/-/tree/main", "git.apps.hexmos.com", "group/repo"},
		{"group/repo", "", "group/repo"},
	}
	for _, tc := range cases {
		h, p := NormalizeRemoteRepoURL(tc.in)
		if h != tc.host || p != tc.path {
			t.Fatalf("NormalizeRemoteRepoURL(%q) = (%q, %q), want (%q, %q)", tc.in, h, p, tc.host, tc.path)
		}
	}
}

func TestRepositoryMatchesRemote(t *testing.T) {
	row := "framework-examples/typescript/nestjs/blive-microservice-2-consumer"
	web := "https://git.apps.hexmos.com/" + row
	if !RepositoryMatchesRemote(row, web, web+".git", "git@git.apps.hexmos.com:"+row+".git", "git@git.apps.hexmos.com:"+row+".git") {
		t.Fatal("expected scp-like remote to match the row")
	}
	if RepositoryMatchesRemote(row, web, "", "", "https://git.apps.hexmos.com/other/repo") {
		t.Fatal("unrelated repo must not match")
	}
	if RepositoryMatchesRemote(row, web, "", "", "") {
		t.Fatal("empty remote must not match")
	}
}

func TestGraphCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := &serverscore.RepoGraph{
		Project:  "proj",
		Routes:   []serverscore.RouteNode{{QualifiedName: "__route__GET__/x", Name: "/x", Method: "GET", FilePath: "src/x.ts"}},
		Handlers: map[string][]string{"__route__GET__/x": {"pkg.X.get"}},
		Outbound: map[string][]serverscore.OutboundRoute{"pkg.X.get": {{Name: "/y", QualifiedName: "__route__POST__/y", Method: "POST"}}},
	}
	writeGraphCache(dir, "proj", g)
	got, ok := readGraphCache(dir, "proj")
	if !ok {
		t.Fatal("expected the graph cache to round-trip")
	}
	if got.Project != "proj" || len(got.Routes) != 1 || got.Routes[0].Method != "GET" {
		t.Fatalf("routes = %+v", got.Routes)
	}
	if got.Handlers["__route__GET__/x"][0] != "pkg.X.get" {
		t.Fatalf("handlers = %+v", got.Handlers)
	}
	if got.Outbound["pkg.X.get"][0].Method != "POST" {
		t.Fatalf("outbound = %+v", got.Outbound)
	}
	if _, ok := readGraphCache(dir, "other"); ok {
		t.Fatal("a cache for a different project must not match")
	}
}
