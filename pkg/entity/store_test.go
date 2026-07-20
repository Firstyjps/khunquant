package entity

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"BlackRock":        "blackrock",
		"BlackRock (IBIT)": "blackrock-ibit",
		"  Binance  ":      "binance",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoreUpsertRemovePersist(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	addrs := []Address{
		{Chain: "bitcoin", Address: "34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo", Label: "cold 1"},
		{Chain: "Ethereum", Address: "0xDE0B295669a9FD93d5F28D9Ec85E40f4cb697BAE"},
	}
	e, added, err := s.Upsert("BlackRock", "", "seed", addrs)
	if err != nil {
		t.Fatal(err)
	}
	if e.Slug != "blackrock" || added != 2 {
		t.Fatalf("upsert: slug=%s added=%d", e.Slug, added)
	}

	// Duplicate (case-insensitive EVM) must not double-add.
	_, added, err = s.Upsert("BlackRock", "", "", []Address{
		{Chain: "ethereum", Address: "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae"},
	})
	if err != nil || added != 0 {
		t.Fatalf("dup add: added=%d err=%v", added, err)
	}

	// Reload from disk.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("blackrock")
	if !ok || len(got.Addresses) != 2 {
		t.Fatalf("reload: ok=%v addrs=%d", ok, len(got.Addresses))
	}

	// Label lookup: registered entity wins with label suffix.
	if l := s2.LabelFor("bitcoin", "34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo"); l != "BlackRock · cold 1" {
		t.Errorf("LabelFor = %q", l)
	}
	// Built-in seed for unregistered address.
	if l := s2.LabelFor("bitcoin", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"); l != "Bitcoin genesis (Satoshi)" {
		t.Errorf("seed LabelFor = %q", l)
	}
	if l := s2.LabelFor("bitcoin", "1UnknownAddrxxxxxxxxxxxxxxxxxxxxx"); l != "" {
		t.Errorf("unknown LabelFor = %q", l)
	}

	// Remove one address, then the entity.
	removed, err := s2.Remove("blackrock", "ethereum", "0xDE0B295669A9FD93D5F28D9EC85E40F4CB697BAE")
	if err != nil || !removed {
		t.Fatalf("remove addr: %v removed=%v", err, removed)
	}
	got, _ = s2.Get("blackrock")
	if len(got.Addresses) != 1 {
		t.Fatalf("after addr remove: %d", len(got.Addresses))
	}
	removed, err = s2.Remove("blackrock", "", "")
	if err != nil || !removed {
		t.Fatalf("remove entity: %v removed=%v", err, removed)
	}
	if _, ok := s2.Get("blackrock"); ok {
		t.Fatal("entity still present after remove")
	}
}

func TestIsBTCAddress(t *testing.T) {
	good := []string{
		"34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo",
		"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
		"bc1qwelntg7tpxwgmh7gea0kycclx87mksnvhaadgf",
	}
	for _, a := range good {
		if !IsBTCAddress(a) {
			t.Errorf("IsBTCAddress(%q) = false", a)
		}
	}
	bad := []string{"", "0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae", "bc1", "2NEWaddressTestnet0000000000000000"}
	for _, a := range bad {
		if IsBTCAddress(a) {
			t.Errorf("IsBTCAddress(%q) = true", a)
		}
	}
}
