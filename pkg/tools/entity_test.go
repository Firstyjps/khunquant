package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/entity"
)

func newEntityStore(t *testing.T) *entity.Store {
	t.Helper()
	s, err := entity.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEntityAddListRemove(t *testing.T) {
	store := newEntityStore(t)
	add := NewEntityAddTool(store)
	ctx := context.Background()

	res := add.Execute(ctx, map[string]any{
		"name": "BlackRock",
		"note": "seed from Arkham",
		"addresses": []any{
			map[string]any{"chain": "btc", "address": "34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo", "label": "custody 1"},
			map[string]any{"chain": "ethereum", "address": "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae"},
		},
	})
	if res.IsError {
		t.Fatalf("entity_add: %s", res.ContentForLLM())
	}
	if !strings.Contains(res.ContentForLLM(), "slug: blackrock") {
		t.Errorf("unexpected output: %s", res.ContentForLLM())
	}

	// "btc" must normalize to "bitcoin".
	e, ok := store.Get("blackrock")
	if !ok || e.Addresses[0].Chain != "bitcoin" {
		t.Fatalf("chain normalize failed: %+v", e.Addresses)
	}

	list := NewEntityListTool(store).Execute(ctx, nil)
	if !strings.Contains(list.ContentForLLM(), "BlackRock") || !strings.Contains(list.ContentForLLM(), "bitcoin") {
		t.Errorf("entity_list output: %s", list.ContentForLLM())
	}

	rm := NewEntityRemoveTool(store).Execute(ctx, map[string]any{"slug": "blackrock"})
	if rm.IsError {
		t.Fatalf("entity_remove: %s", rm.ContentForLLM())
	}
	if _, ok := store.Get("blackrock"); ok {
		t.Fatal("entity still present")
	}
}

func TestEntityAddRejectsBadAddress(t *testing.T) {
	store := newEntityStore(t)
	res := NewEntityAddTool(store).Execute(context.Background(), map[string]any{
		"name": "Bad",
		"addresses": []any{
			map[string]any{"chain": "bitcoin", "address": "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae"},
		},
	})
	if !res.IsError {
		t.Fatal("expected error for EVM address on bitcoin chain")
	}
}

func TestEntityHoldingsUnknownSlug(t *testing.T) {
	store := newEntityStore(t)
	res := NewEntityHoldingsTool(store).Execute(context.Background(), map[string]any{"slug": "nope"})
	if !res.IsError {
		t.Fatal("expected error for unknown slug")
	}
}

func TestEntityFlowsNoBTCAddresses(t *testing.T) {
	store := newEntityStore(t)
	if _, _, err := store.Upsert("EthOnly", "", "", []entity.Address{
		{Chain: "ethereum", Address: "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae"},
	}); err != nil {
		t.Fatal(err)
	}
	res := NewEntityFlowsTool(store).Execute(context.Background(), map[string]any{"slug": "ethonly"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.ContentForLLM())
	}
	if !strings.Contains(res.ContentForLLM(), "Bitcoin only") {
		t.Errorf("output: %s", res.ContentForLLM())
	}
}
