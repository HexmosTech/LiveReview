package api

import (
	"testing"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/blastradius/crossrepolink"
)

func TestNormalizeCrossRepoSymbolsMergesAndValidates(t *testing.T) {
	got, err := normalizeCrossRepoSymbols([]CrossRepoSymbolCalls{
		{Key: "a", Calls: []crossrepolink.Call{{Method: "post", Path: "/v1/x"}}},
		{Key: "a", Calls: []crossrepolink.Call{{Method: "GET", Path: "/v1/y"}}},
		{Key: "b", Calls: []crossrepolink.Call{{Path: " /z "}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "a" || got[1].Key != "b" {
		t.Fatalf("order = %+v", got)
	}
	if len(got[0].Calls) != 2 || got[0].Calls[0].Method != "POST" {
		t.Fatalf("merged calls = %+v", got[0].Calls)
	}
	if got[1].Calls[0].Path != "/z" {
		t.Fatalf("trimmed path = %q", got[1].Calls[0].Path)
	}
}

func TestNormalizeCrossRepoSymbolsRejectsBadInput(t *testing.T) {
	if _, err := normalizeCrossRepoSymbols(nil); err == nil {
		t.Fatal("empty symbols should be rejected")
	}
	if _, err := normalizeCrossRepoSymbols([]CrossRepoSymbolCalls{{Key: " "}}); err == nil {
		t.Fatal("empty key should be rejected")
	}
	calls := make([]crossrepolink.Call, crossRepoMaxCalls+1)
	for i := range calls {
		calls[i] = crossrepolink.Call{Path: "/x"}
	}
	if _, err := normalizeCrossRepoSymbols([]CrossRepoSymbolCalls{{Key: "a", Calls: calls}}); err == nil {
		t.Fatal("too many calls should be rejected")
	}
}

func TestApplyImpactCapIsDeterministic(t *testing.T) {
	impacts := map[string][]blastradius.CrossRepoImpact{
		"b": {{CallSite: "r1"}, {CallSite: "r2"}},
		"a": {{CallSite: "r3"}},
	}
	out, dropped := applyImpactCap(impacts)
	if dropped != 0 || len(out["a"]) != 1 || len(out["b"]) != 2 {
		t.Fatalf("cap changed a small response: %+v dropped=%d", out, dropped)
	}

	big := map[string][]blastradius.CrossRepoImpact{}
	for i := 0; i < crossRepoMaxImpacts+10; i++ {
		key := string(rune('a' + i%26))
		big[key] = append(big[key], blastradius.CrossRepoImpact{CallSite: "r"})
	}
	capped, dropped := applyImpactCap(big)
	total := 0
	for _, list := range capped {
		total += len(list)
	}
	if total != crossRepoMaxImpacts || dropped == 0 {
		t.Fatalf("cap total = %d, dropped = %d; want %d and >0", total, dropped, crossRepoMaxImpacts)
	}
}
