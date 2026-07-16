package defi

import (
	"testing"
)

func TestAddressValidation(t *testing.T) {
	if !IsEVMAddress("0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045") {
		t.Error("valid EVM address rejected")
	}
	if IsEVMAddress("0x123") || IsEVMAddress("d8dA6BF26964aF9D7eEd9e03E53415D37aA96045") {
		t.Error("invalid EVM address accepted")
	}
	if !IsSolanaAddress("5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1") {
		t.Error("valid Solana address rejected")
	}
	if IsSolanaAddress("0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045") {
		t.Error("EVM address accepted as Solana")
	}
}

func TestHexToBig(t *testing.T) {
	n, err := hexToBig(`"0x1bc16d674ec80000"`) // 2e18
	if err != nil {
		t.Fatalf("hexToBig: %v", err)
	}
	if got := bigToFloat(n, 18); got != 2.0 {
		t.Errorf("bigToFloat = %g, want 2.0", got)
	}
	if n, err := hexToBig("0x"); err != nil || n.Sign() != 0 {
		t.Errorf("empty hex should be zero, got %v %v", n, err)
	}
}

func TestDecodeABIString(t *testing.T) {
	// Dynamic string encoding of "USDT": offset(32) + len(4) + data.
	dynamic := "0x" +
		"0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000004" +
		"5553445400000000000000000000000000000000000000000000000000000000"
	if got := decodeABIString(dynamic); got != "USDT" {
		t.Errorf("dynamic decode = %q, want USDT", got)
	}
	// bytes32 encoding of "MKR" (legacy tokens like Maker).
	bytes32 := "0x4d4b520000000000000000000000000000000000000000000000000000000000"
	if got := decodeABIString(bytes32); got != "MKR" {
		t.Errorf("bytes32 decode = %q, want MKR", got)
	}
}

func TestLooksLikeLPToken(t *testing.T) {
	for _, sym := range []string{"UNI-V2", "Cake-LP", "SLP", "BTC-USDT-LP"} {
		if !looksLikeLPToken(sym) {
			t.Errorf("%s should be detected as LP", sym)
		}
	}
	for _, sym := range []string{"USDT", "WETH", "SOL"} {
		if looksLikeLPToken(sym) {
			t.Errorf("%s should not be detected as LP", sym)
		}
	}
}

func TestStoreAddRemoveList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	w := Wallet{Chain: "Ethereum", Address: "0xD8DA6BF26964AF9D7EED9E03E53415D37AA96045", Label: "vitalik"}
	if err := s.Add(w); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Duplicate add (different case) must update, not append.
	w.Label = "vitalik2"
	if err := s.Add(w); err != nil {
		t.Fatalf("Add update: %v", err)
	}
	list := s.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 wallet, got %d", len(list))
	}
	if list[0].Label != "vitalik2" {
		t.Errorf("label = %q, want vitalik2", list[0].Label)
	}
	if list[0].Address != "0xd8da6bf26964af9d7eed9e03e53415d37aa96045" {
		t.Errorf("EVM address should be lower-cased, got %q", list[0].Address)
	}

	// Persistence across reload.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore reload: %v", err)
	}
	if len(s2.List()) != 1 {
		t.Fatal("watchlist did not persist")
	}

	removed, err := s2.Remove("ethereum", "0xD8DA6BF26964AF9D7EED9E03E53415D37AA96045")
	if err != nil || !removed {
		t.Fatalf("Remove = (%v, %v), want (true, nil)", removed, err)
	}
	if len(s2.List()) != 0 {
		t.Fatal("wallet not removed")
	}
}
