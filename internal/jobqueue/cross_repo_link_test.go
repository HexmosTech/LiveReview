package jobqueue

import (
	"testing"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/serverscore"
)

func TestRepositoryIdentities(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{
			in:   "https://git.apps.hexmos.com/framework-examples/typescript/nestjs/blive-microservice-2-consumer",
			want: []string{"https://git.apps.hexmos.com/framework-examples/typescript/nestjs/blive-microservice-2-consumer", "framework-examples/typescript/nestjs/blive-microservice-2-consumer"},
		},
		{
			in:   "framework-examples/typescript/nestjs/blive-microservice-2-consumer",
			want: []string{"framework-examples/typescript/nestjs/blive-microservice-2-consumer"},
		},
		{
			in:   "https://git.example.com/group/sub/repo.git",
			want: []string{"https://git.example.com/group/sub/repo.git", "group/sub/repo"},
		},
		{
			in:   "https://git.example.com/group/repo/-/merge_requests/7",
			want: []string{"https://git.example.com/group/repo/-/merge_requests/7", "group/repo"},
		},
	}
	for _, tc := range cases {
		got := repositoryIdentities(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("repositoryIdentities(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("repositoryIdentities(%q)[%d] = %q, want %q (full: %v)", tc.in, i, got[i], tc.want[i], got)
			}
		}
	}
}

func TestVersionAliasedImpacts_MatchesVersionedCallToUnversionedRoute(t *testing.T) {
	main := &serverscore.RepoGraph{
		Project: "main",
		Outbound: map[string][]serverscore.OutboundRoute{
			"main.sym": {{Name: "/v1/sms/send-otp", QualifiedName: "__route__POST__/v1/sms/send-otp"}},
		},
	}
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes: []serverscore.RouteNode{
			{QualifiedName: "__route__POST__/sms/send-otp", Name: "/sms/send-otp", FilePath: "src/sms.controller.ts"},
			{QualifiedName: "__route__POST__/unrelated", Name: "/unrelated", FilePath: "src/other.controller.ts"},
		},
	}
	got := versionAliasedImpacts(main, map[string]*serverscore.RepoGraph{"main": main, "target": target}, []string{"main.sym"}, nil)
	impacts := got["main.sym"]
	if len(impacts) != 1 {
		t.Fatalf("versionAliasedImpacts = %+v, want one match", impacts)
	}
	if impacts[0].RelatedRepo != "target" || impacts[0].CallSite != "__route__POST__/sms/send-otp" || impacts[0].MatchKind != "version-alias" {
		t.Fatalf("unexpected impact: %+v", impacts[0])
	}
}

func TestVersionAliasedImpacts_SuppressedWhenExactExists(t *testing.T) {
	main := &serverscore.RepoGraph{
		Project: "main",
		Outbound: map[string][]serverscore.OutboundRoute{
			"main.sym": {{Name: "/v1/e-kyc/read-aadhaar"}},
		},
	}
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes:  []serverscore.RouteNode{{QualifiedName: "__route__POST__/e-kyc/read-aadhaar", Name: "/e-kyc/read-aadhaar", FilePath: "src/e-kyc.controller.ts"}},
	}
	exact := map[string]map[string]bool{"main.sym": {"/e-kyc/read-aadhaar": true}}
	got := versionAliasedImpacts(main, map[string]*serverscore.RepoGraph{"main": main, "target": target}, []string{"main.sym"}, exact)
	if len(got["main.sym"]) != 0 {
		t.Fatalf("fallback must not add a path-colliding repo when exact already matched: %+v", got["main.sym"])
	}
}

func TestExactCallNorms(t *testing.T) {
	got := exactCallNorms(map[string][]blastradius.CrossRepoImpact{
		"sym": {{CallSite: "__route__POST__/v1/e-kyc/read-aadhaar"}},
	})
	if !got["sym"]["/e-kyc/read-aadhaar"] {
		t.Fatalf("exactCallNorms = %+v, want /e-kyc/read-aadhaar", got)
	}
	if routeNameFromCallSite("__route__GET__/v1/orders") != "/v1/orders" {
		t.Fatalf("routeNameFromCallSite = %q", routeNameFromCallSite("__route__GET__/v1/orders"))
	}
}

func TestVersionAliasedImpacts_DoesNotMatchSameVersioning(t *testing.T) {
	main := &serverscore.RepoGraph{
		Project: "main",
		Outbound: map[string][]serverscore.OutboundRoute{
			"main.sym": {{Name: "/v1/orders"}},
		},
	}
	target := &serverscore.RepoGraph{
		Project: "target",
		Routes:  []serverscore.RouteNode{{QualifiedName: "__route__GET__/v1/orders", Name: "/v1/orders", FilePath: "src/orders.controller.ts"}},
	}
	got := versionAliasedImpacts(main, map[string]*serverscore.RepoGraph{"main": main, "target": target}, []string{"main.sym"}, nil)
	if len(got["main.sym"]) != 0 {
		t.Fatalf("expected no fallback when both sides are versioned: %+v", got["main.sym"])
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
	meta := map[string]repoMeta{
		"svc-b": {ID: 2, FullName: "org/b", WebURL: "https://git/org/b"},
		"svc-c": {ID: 3, FullName: "org/c", WebURL: "https://git/org/c"},
	}
	impacts := []blastradius.CrossRepoImpact{{
		RelatedRepo: "svc-b", RelatedRepoID: 2, RelatedRepoName: "org/b",
		CallSite: "__route__POST__/orders", EdgeType: "CROSS_HTTP_CALLS",
	}}
	chains := buildChains(main, map[string]*serverscore.RepoGraph{"main": main, "svc-b": b, "svc-c": c}, meta, impacts, 3)
	if len(chains) != 1 {
		t.Fatalf("buildChains = %+v, want one chain", chains)
	}
	hops := chains[0].Hops
	if len(hops) != 2 {
		t.Fatalf("chain hops = %+v, want [svc-b, svc-c]", hops)
	}
	if hops[0].RelatedRepo != "svc-b" || hops[1].RelatedRepo != "svc-c" {
		t.Fatalf("chain order = %+v", hops)
	}
	if hops[1].RelatedRepoName != "org/c" || hops[1].CallSite != "__route__POST__/ledger/entry" {
		t.Fatalf("second hop = %+v", hops[1])
	}
}

func TestRelatedRepoGraph_HandlesCycleAndCap(t *testing.T) {
	// 1 -> 2 -> 3 -> 1 is a legal relation cycle (the schema forbids only
	// self-links and duplicate pairs). BFS must visit each repo once.
	adj := map[int64][]int64{
		1: {2},
		2: {1, 3},
		3: {2, 1},
	}
	got := relatedRepoGraph(adj, 1, 3, 10)
	if len(got) != 2 {
		t.Fatalf("relatedRepoGraph = %+v, want repos 2 and 3 once each", got)
	}
	if got[0].id != 2 || got[0].depth != 1 || got[1].id != 3 || got[1].depth != 2 {
		t.Fatalf("relatedRepoGraph order/depth = %+v", got)
	}

	// The cap must still hold even when the first hop is wide.
	wide := map[int64][]int64{1: {2, 3, 4, 5}}
	capped := relatedRepoGraph(wide, 1, 3, 2)
	if len(capped) != 2 {
		t.Fatalf("relatedRepoGraph cap = %+v, want 2", capped)
	}
}

func TestBuildChains_CycleDoesNotLoop(t *testing.T) {
	// main <- B -> C -> B and B -> main are both cycles. The walker must not
	// revisit main or B, and must not emit duplicate/reversed chains.
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
	meta := map[string]repoMeta{"svc-b": {ID: 2, FullName: "org/b"}, "svc-c": {ID: 3, FullName: "org/c"}}
	impacts := []blastradius.CrossRepoImpact{{RelatedRepo: "svc-b", RelatedRepoID: 2, RelatedRepoName: "org/b", CallSite: "__route__POST__/orders", EdgeType: "CROSS_HTTP_CALLS"}}
	chains := buildChains(main, graphs, meta, impacts, 3)
	if len(chains) != 1 {
		t.Fatalf("buildChains with cycles = %+v, want exactly one [svc-b, svc-c] chain", chains)
	}
	if len(chains[0].Hops) != 2 || chains[0].Hops[0].RelatedRepo != "svc-b" || chains[0].Hops[1].RelatedRepo != "svc-c" {
		t.Fatalf("cycle chain = %+v", chains[0].Hops)
	}
}
