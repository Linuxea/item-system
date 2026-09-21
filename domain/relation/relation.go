package relation

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/event"
)

type Type string

const (
	TypeCP     Type = "cp"
	TypeMaster Type = "master"
	TypeBestie Type = "bestie"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusDissolved Status = "dissolved"
)

var (
	ErrNotFound     = errors.New("relation not found")
	ErrSelfBinding  = errors.New("cannot bind relation to self")
	ErrAlreadyBound = errors.New("relation already bound")
	ErrDissolved    = errors.New("relation dissolved")
)

type Relation struct {
	ID        string
	Type      Type
	PartyA    string
	PartyB    string
	Status    Status
	CreatedAt time.Time
	ExpireAt  *time.Time
	Version   int64
}

func (r *Relation) Expired(now time.Time) bool {
	return r.ExpireAt != nil && !now.Before(*r.ExpireAt)
}

func (r *Relation) Involves(owner string) bool {
	return r.PartyA == owner || r.PartyB == owner
}

func (r *Relation) BumpVersion() int64 {
	r.Version++
	return r.Version
}

const (
	EventBound     = "relation.bound"
	EventDissolved = "relation.dissolved"
)

type Bound struct {
	event.Base
	RelationID string
	Type       Type
	PartyA     string
	PartyB     string
}

func (e Bound) Name() string { return EventBound }

type Dissolved struct {
	event.Base
	RelationID string
	Type       Type
	PartyA     string
	PartyB     string
	ByExpiry   bool
}

func (e Dissolved) Name() string { return EventDissolved }

type Repo interface {
	Save(ctx context.Context, r *Relation) error
	Get(ctx context.Context, id string) (*Relation, error)
	FindActive(ctx context.Context, owner string, t Type) (*Relation, error)
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*Relation, error)
	Update(ctx context.Context, r *Relation, expectVersion int64) error
}

type Clock func() time.Time

type Service struct {
	repo      Repo
	publisher event.Publisher
	newID     func() string
	now       Clock
}

func NewService(repo Repo, publisher event.Publisher, newID func() string, now Clock) *Service {
	return &Service{repo: repo, publisher: publisher, newID: newID, now: now}
}

func (s *Service) Bind(ctx context.Context, t Type, partyA, partyB string, expireAt *time.Time) (*Relation, error) {
	if partyA == partyB {
		return nil, ErrSelfBinding
	}
	for _, party := range []string{partyA, partyB} {
		if _, err := s.repo.FindActive(ctx, party, t); !errors.Is(err, ErrNotFound) {
			return nil, ErrAlreadyBound
		}
	}

	r := &Relation{
		ID:        s.newID(),
		Type:      t,
		PartyA:    partyA,
		PartyB:    partyB,
		Status:    StatusActive,
		CreatedAt: s.now(),
		ExpireAt:  expireAt,
	}
	if err := s.repo.Save(ctx, r); err != nil {
		return nil, err
	}

	s.publishBound(r)
	return r, nil
}

func (s *Service) Dissolve(ctx context.Context, id string) error {
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.Status == StatusDissolved {
		return ErrDissolved
	}
	expect := r.Version
	r.Status = StatusDissolved
	r.ExpireAt = nil
	r.BumpVersion()
	if err := s.repo.Update(ctx, r, expect); err != nil {
		return err
	}
	s.publishDissolved(r, false)
	return nil
}

func (s *Service) RunExpiry(ctx context.Context, limit int) (int, error) {
	expired, err := s.repo.ListExpired(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, r := range expired {
		expect := r.Version
		r.Status = StatusDissolved
		r.ExpireAt = nil
		r.BumpVersion()
		if err := s.repo.Update(ctx, r, expect); err != nil {
			return processed, err
		}
		s.publishDissolved(r, true)
		processed++
	}
	return processed, nil
}

func (s *Service) publishBound(r *Relation) {
	s.publisher.Publish(Bound{
		Base:       event.Base{At: s.now()},
		RelationID: r.ID,
		Type:       r.Type,
		PartyA:     r.PartyA,
		PartyB:     r.PartyB,
	})
}

func (s *Service) publishDissolved(r *Relation, byExpiry bool) {
	s.publisher.Publish(Dissolved{
		Base:       event.Base{At: s.now()},
		RelationID: r.ID,
		Type:       r.Type,
		PartyA:     r.PartyA,
		PartyB:     r.PartyB,
		ByExpiry:   byExpiry,
	})
}
