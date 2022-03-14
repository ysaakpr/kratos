package code

//go:generate mockgen -destination=mocks/mock_service.go -package=mocks github.com/ory/kratos/selfservice/strategy/code Flow

import (
	"context"
	"github.com/gofrs/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/ory/kratos/courier"
	templates "github.com/ory/kratos/courier/template/sms"
	"github.com/ory/kratos/driver/clock"
	"github.com/ory/kratos/driver/config"
	"github.com/ory/x/httpx"
)

type Flow interface {
	GetID() uuid.UUID
	Valid() error
}

type AuthenticationService interface {
	SendCode(ctx context.Context, flow Flow, phone string) error
	VerifyCode(ctx context.Context, flow Flow, code string) (*Code, error)
}

type dependencies interface {
	config.Provider
	clock.Provider
	CodePersistenceProvider
	courier.Provider
	courier.ConfigProvider
	HTTPClient(ctx context.Context, opts ...httpx.ResilientOptions) *retryablehttp.Client
	RandomCodeGeneratorProvider
}

type authenticationServiceImpl struct {
	r dependencies
}

type AuthenticationServiceProvider interface {
	CodeAuthenticationService() AuthenticationService
}

func NewCodeAuthenticationService(r dependencies) AuthenticationService {
	return &authenticationServiceImpl{r}
}

// SendCode
// Sends a new code to the user in a message.
// Returns error if the code was already sent and is not expired yet.
func (s *authenticationServiceImpl) SendCode(ctx context.Context, flow Flow, identifier string) error {
	if err := flow.Valid(); err != nil {
		return err
	}

	if err := s.r.CodePersister().DeleteCodes(ctx, identifier); err != nil {
		return err
	}

	codeValue := ""
	sendSMS := true
	for _, n := range s.r.Config(ctx).SelfServiceCodeTestNumbers() {
		if n == identifier {
			codeValue = "0000"
			sendSMS = false
			break
		}
	}

	if sendSMS {
		codeValue = s.r.RandomCodeGenerator().Generate(4)
	}

	if err := s.r.CodePersister().CreateCode(ctx, &Code{
		FlowId:     flow.GetID(),
		Identifier: identifier,
		Code:       codeValue,
		ExpiresAt:  s.r.Clock().Now().Add(s.r.Config(ctx).SelfServiceCodeLifespan()),
	}); err != nil {
		return err
	}

	if sendSMS {
		if _, err := s.r.Courier(ctx).QueueSMS(
			ctx,
			templates.NewCodeMessage(s.r, &templates.CodeMessageModel{Code: codeValue, To: identifier}),
		); err != nil {
			return err
		}
	}
	return nil
}

// VerifyCode
// Verifies code by looking up in db.
func (s *authenticationServiceImpl) VerifyCode(ctx context.Context, flow Flow, code string) (*Code, error) {
	if err := flow.Valid(); err != nil {
		return nil, err
	}
	expectedCode, err := s.r.CodePersister().FindActiveCode(ctx, flow.GetID(), s.r.Clock().Now())
	if err != nil {
		return nil, err
	}
	if expectedCode == nil || expectedCode.Code != code {
		return nil, NewInvalidCodeError()
	} else if expectedCode.Attempts >= s.r.Config(ctx).SelfServiceCodeMaxAttempts() {
		return nil, NewAttemptsExceededError()
	}

	return expectedCode, nil
}
