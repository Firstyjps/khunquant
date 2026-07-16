package defi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cryptoquantumwave/khunquant/pkg/fileutil"
)

// Wallet is one tracked wallet.
type Wallet struct {
	Chain   string   `json:"chain"`            // "ethereum", "bsc", ..., "solana"
	Address string   `json:"address"`          // 0x… or base58
	Label   string   `json:"label,omitempty"`  // human name, e.g. "hot wallet"
	Tokens  []string `json:"tokens,omitempty"` // extra ERC-20 contracts to watch (EVM only)
}

// Store persists the tracked-wallet watchlist as JSON under
// {workspace}/memory/defi/wallets.json.
type Store struct {
	path    string
	mu      sync.Mutex
	wallets []Wallet
}

// NewStore loads (or initializes) the watchlist store.
func NewStore(workspacePath string) (*Store, error) {
	dir := filepath.Join(workspacePath, "memory", "defi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("defi store: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "wallets.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("defi store read: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &s.wallets); err != nil {
		return fmt.Errorf("defi store parse: %w", err)
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.wallets, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(s.path, data, 0o600)
}

func normalizeWalletKey(chain, address string) (string, string) {
	chain = strings.ToLower(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	if chain != "solana" {
		address = strings.ToLower(address)
	}
	return chain, address
}

// Add registers a wallet; updating label/tokens if it already exists.
func (s *Store) Add(w Wallet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	chain, addr := normalizeWalletKey(w.Chain, w.Address)
	w.Chain, w.Address = chain, addr
	for i, existing := range s.wallets {
		ec, ea := normalizeWalletKey(existing.Chain, existing.Address)
		if ec == chain && ea == addr {
			s.wallets[i] = w
			return s.save()
		}
	}
	s.wallets = append(s.wallets, w)
	return s.save()
}

// Remove deletes a wallet from the watchlist.
func (s *Store) Remove(chain, address string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chain, addr := normalizeWalletKey(chain, address)
	for i, existing := range s.wallets {
		ec, ea := normalizeWalletKey(existing.Chain, existing.Address)
		if ec == chain && ea == addr {
			s.wallets = append(s.wallets[:i], s.wallets[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

// List returns a copy of the tracked wallets.
func (s *Store) List() []Wallet {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Wallet, len(s.wallets))
	copy(out, s.wallets)
	return out
}
