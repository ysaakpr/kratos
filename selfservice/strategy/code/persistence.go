package code

//go:generate mockgen -destination=mocks/mock_persistence.go -package=mocks github.com/ory/kratos/selfservice/strategy/code CodePersister

import (
	"context"
	"github.com/gofrs/uuid"
	"time"
)

type CodePersister interface {
	CreateCode(ctx context.Context, code *Code) error

	// FindActiveCode selects code by login flow id and expiration date/time.
	FindActiveCode(ctx context.Context, flowId uuid.UUID, expiresAfter time.Time) (*Code, error)
	// DeleteCodes deletes all codes with the given identifier
	DeleteCodes(ctx context.Context, identifier string) error
}

type CodePersistenceProvider interface {
	CodePersister() CodePersister
}
