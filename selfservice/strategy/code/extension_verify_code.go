package code

import (
	"github.com/ory/jsonschema/v3"
	"github.com/ory/kratos/schema"
	"sync"
)

type SchemaExtensionVerification struct {
	l          sync.Mutex
	identifier string
}

func NewSchemaExtensionVerificationCode(verifiedIdentifier string) *SchemaExtensionVerification {
	return &SchemaExtensionVerification{identifier: verifiedIdentifier}
}

func (r *SchemaExtensionVerification) Run(ctx jsonschema.ValidationContext, s schema.ExtensionConfig, value interface{}) error {
	r.l.Lock()
	defer r.l.Unlock()

	if s.Credentials.Code.Identifier {
		if value != r.identifier {
			return ctx.Error("", "phone number in identity traits not equal to verified phone")
		}
	}

	return nil
}

func (r *SchemaExtensionVerification) Finish() error {
	return nil
}
