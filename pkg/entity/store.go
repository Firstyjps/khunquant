// Package entity provides an Arkham-style on-chain entity tracker: named
// entities (funds, exchanges, whales) each own a set of addresses across
// Bitcoin, EVM chains, and Solana. Holdings and transaction flows are read
// from keyless public data sources (Esplora for Bitcoin, JSON-RPC via the
// defi package for the rest).
package entity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/fileutil"
)

// Address is one tracked address belonging to an entity.
type Address struct {
	Chain   string   `json:"chain"`            // "bitcoin", "ethereum", ..., "solana"
	Address string   `json:"address"`          // bc1…/1…/3…, 0x…, or base58
	Label   string   `json:"label,omitempty"`  // e.g. "IBIT custody 1"
	Tokens  []string `json:"tokens,omitempty"` // extra ERC-20 contracts to check (EVM only)
}

// Entity is a named owner of one or more on-chain addresses.
type Entity struct {
	Slug      string    `json:"slug"` // stable id, e.g. "blackrock"
	Name      string    `json:"name"` // display name, e.g. "BlackRock"
	Note      string    `json:"note,omitempty"`
	Addresses []Address `json:"addresses"`
	UpdatedAt time.Time `json:"updated_at"`
}

// wellKnownLabels seeds counterparty labeling for a few thoroughly verified
// public addresses. Deliberately tiny: a wrong label in a money bot is worse
// than no label. User-registered entities always take precedence.
var wellKnownLabels = map[string]string{
	"bitcoin:34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo":          "Binance cold wallet",
	"bitcoin:1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa":          "Bitcoin genesis (Satoshi)",
	"ethereum:0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae": "Ethereum Foundation",
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify derives a stable slug from a display name ("BlackRock (IBIT)" →
// "blackrock-ibit").
func Slugify(name string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	return strings.Trim(s, "-")
}

// Store persists the entity registry as JSON under
// {workspace}/memory/entity/registry.json.
type Store struct {
	path     string
	mu       sync.Mutex
	entities []Entity
}

// NewStore loads (or initializes) the entity registry.
func NewStore(workspacePath string) (*Store, error) {
	dir := filepath.Join(workspacePath, "memory", "entity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("entity store: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "registry.json")}
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
		return fmt.Errorf("entity store read: %w", err)
	}
	if err := json.Unmarshal(data, &s.entities); err != nil {
		return fmt.Errorf("entity store parse: %w", err)
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.entities, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(s.path, data, 0o644)
}

func addrKey(chain, address string) string {
	chain = strings.ToLower(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	// EVM addresses are case-insensitive hex; Bitcoin legacy/base58 are not.
	if strings.HasPrefix(address, "0x") {
		address = strings.ToLower(address)
	}
	return chain + ":" + address
}

// Upsert creates the entity if missing (by slug) and merges the given
// addresses into it, deduplicating by chain+address. Returns the stored
// entity and how many addresses were newly added.
func (s *Store) Upsert(name, slug, note string, addrs []Address) (Entity, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slug == "" {
		slug = Slugify(name)
	}
	if slug == "" {
		return Entity{}, 0, fmt.Errorf("entity needs a non-empty name")
	}

	idx := -1
	for i := range s.entities {
		if s.entities[i].Slug == slug {
			idx = i
			break
		}
	}
	if idx == -1 {
		s.entities = append(s.entities, Entity{Slug: slug, Name: strings.TrimSpace(name)})
		idx = len(s.entities) - 1
	}
	e := &s.entities[idx]
	if note != "" {
		e.Note = note
	}
	if e.Name == "" {
		e.Name = slug
	}

	existing := map[string]bool{}
	for _, a := range e.Addresses {
		existing[addrKey(a.Chain, a.Address)] = true
	}
	added := 0
	for _, a := range addrs {
		a.Chain = strings.ToLower(strings.TrimSpace(a.Chain))
		a.Address = strings.TrimSpace(a.Address)
		k := addrKey(a.Chain, a.Address)
		if a.Address == "" || existing[k] {
			continue
		}
		existing[k] = true
		e.Addresses = append(e.Addresses, a)
		added++
	}
	e.UpdatedAt = time.Now().UTC()
	stored := *e
	if err := s.save(); err != nil {
		return Entity{}, 0, err
	}
	return stored, added, nil
}

// Remove deletes a whole entity (address == "") or a single address from it.
// Returns whether anything was removed.
func (s *Store) Remove(slug, chain, address string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.entities {
		if s.entities[i].Slug != slug {
			continue
		}
		if strings.TrimSpace(address) == "" {
			s.entities = append(s.entities[:i], s.entities[i+1:]...)
			return true, s.save()
		}
		k := addrKey(chain, address)
		e := &s.entities[i]
		for j := range e.Addresses {
			if addrKey(e.Addresses[j].Chain, e.Addresses[j].Address) == k {
				e.Addresses = append(e.Addresses[:j], e.Addresses[j+1:]...)
				e.UpdatedAt = time.Now().UTC()
				return true, s.save()
			}
		}
		return false, nil
	}
	return false, nil
}

// Get returns an entity by slug.
func (s *Store) Get(slug string) (Entity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entities {
		if e.Slug == slug {
			return e, true
		}
	}
	return Entity{}, false
}

// List returns all entities sorted by slug.
func (s *Store) List() []Entity {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entity, len(s.entities))
	copy(out, s.entities)
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// LabelFor resolves a counterparty address to a human label: registered
// entities first ("EntityName · addr label"), then the built-in well-known
// seed. Returns "" when unknown.
func (s *Store) LabelFor(chain, address string) string {
	k := addrKey(chain, address)
	s.mu.Lock()
	for _, e := range s.entities {
		for _, a := range e.Addresses {
			if addrKey(a.Chain, a.Address) == k {
				s.mu.Unlock()
				if a.Label != "" {
					return e.Name + " · " + a.Label
				}
				return e.Name
			}
		}
	}
	s.mu.Unlock()
	return wellKnownLabels[k]
}
