package rebalance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/fileutil"
)

// Store persists rebalancing plans as JSON under
// {workspace}/memory/rebalance/plans.json. Plans are few and have no
// per-run history table (history lives in portfolio snapshots), so a
// flat JSON file with atomic writes is sufficient.
type Store struct {
	path   string
	mu     sync.Mutex
	nextID int64
	plans  []Plan
}

// NewStore loads (or initializes) the plan store.
func NewStore(workspacePath string) (*Store, error) {
	dir := filepath.Join(workspacePath, "memory", "rebalance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("rebalance store: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "plans.json"), nextID: 1}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

type storeFile struct {
	NextID int64  `json:"next_id"`
	Plans  []Plan `json:"plans"`
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("rebalance store read: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var f storeFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("rebalance store parse: %w", err)
	}
	s.plans = f.Plans
	s.nextID = f.NextID
	if s.nextID < 1 {
		s.nextID = 1
	}
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(storeFile{NextID: s.nextID, Plans: s.plans}, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(s.path, data, 0o600)
}

// Save inserts a new plan (assigning ID) or updates an existing one by ID.
func (s *Store) Save(p *Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	p.UpdatedAtMS = now
	if p.ID == 0 {
		p.ID = s.nextID
		s.nextID++
		p.CreatedAtMS = now
		s.plans = append(s.plans, *p)
		return s.save()
	}
	for i := range s.plans {
		if s.plans[i].ID == p.ID {
			s.plans[i] = *p
			return s.save()
		}
	}
	return fmt.Errorf("rebalance plan %d not found", p.ID)
}

// Get returns a plan by ID.
func (s *Store) Get(id int64) (Plan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.plans {
		if p.ID == id {
			return p, true
		}
	}
	return Plan{}, false
}

// FindByName returns a plan by (case-insensitive) name.
func (s *Store) FindByName(name string) (Plan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.plans {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Plan{}, false
}

// Delete removes a plan by ID.
func (s *Store) Delete(id int64) (Plan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.plans {
		if p.ID == id {
			s.plans = append(s.plans[:i], s.plans[i+1:]...)
			return p, true, s.save()
		}
	}
	return Plan{}, false, nil
}

// List returns a copy of all plans.
func (s *Store) List() []Plan {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Plan, len(s.plans))
	copy(out, s.plans)
	return out
}
