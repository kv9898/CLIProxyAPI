package clientkeys

import (
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"sync"
	"testing"
)

func TestCounterConcurrentAndEphemeral(t *testing.T) {
	counter := NewCounter()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			counter.Record(coreusage.Record{APIKey: "key-one", Detail: coreusage.Detail{InputTokens: 8, OutputTokens: 2, CachedTokens: 3, ReasoningTokens: 1, TotalTokens: 10}})
		})
	}
	wg.Wait()
	counter.Record(coreusage.Record{APIKey: "key-two", Detail: coreusage.Detail{TotalTokens: 250}})
	counter.Record(coreusage.Record{APIKey: "", Detail: coreusage.Detail{TotalTokens: 999}})
	snapshot := counter.Snapshot()
	if snapshot.Total != 1250 || snapshot.Keys[ID("key-one")] != 1000 || snapshot.Keys[ID("key-two")] != 250 {
		t.Fatalf("incorrect totals: %+v", snapshot)
	}
	snapshot.Keys[ID("key-one")] = 0
	if counter.Snapshot().Keys[ID("key-one")] != 1000 {
		t.Fatal("snapshot aliases counter")
	}
	if NewCounter().Snapshot().Total != 0 {
		t.Fatal("counter persisted")
	}
}
