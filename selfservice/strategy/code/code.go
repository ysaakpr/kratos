package code

import (
	"context"
	"github.com/gofrs/uuid"
	"github.com/ory/kratos/corp"
	"time"
)

type Code struct {
	ID         uuid.UUID `json:"-" faker:"-" db:"id"`
	FlowId     uuid.UUID `json:"-" faker:"-" db:"flow_id"`
	Identifier string    `json:"-" faker:"phone_number" db:"identifier"`
	Code       string    `json:"-" db:"code"`
	ExpiresAt  time.Time `json:"-" faker:"time_type" db:"expires_at"`
	Attempts   int       `json:"-" faker:"-" db:"attempts"`

	// CreatedAt is a helper struct field for gobuffalo.pop.
	CreatedAt time.Time `json:"-" faker:"-" db:"created_at"`
	// UpdatedAt is a helper struct field for gobuffalo.pop.
	UpdatedAt time.Time `json:"-" faker:"-" db:"updated_at"`
}

func (m Code) TableName(ctx context.Context) string {
	return corp.ContextualizeTableName(ctx, "auth_codes")
}

func (m *Code) GetID() uuid.UUID {
	return m.ID
}
