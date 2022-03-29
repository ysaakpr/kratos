package code_test

import (
	"context"
	"github.com/benbjohnson/clock"
	"github.com/gofrs/uuid"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/ory/kratos/courier"
	courierMock "github.com/ory/kratos/courier/mocks"
	"github.com/ory/kratos/driver/config"
	"github.com/ory/kratos/internal"
	"github.com/ory/kratos/selfservice/flow"
	"github.com/ory/kratos/selfservice/strategy/code"
	codeMock "github.com/ory/kratos/selfservice/strategy/code/mocks"
	"github.com/ory/x/httpx"
	"github.com/pkg/errors"
	"log"
	"testing"
	"time"
)

type testContext struct {
	context    context.Context
	controller *gomock.Controller
	config     *config.Config
}

func TestAuthenticationService_SendCode(t *testing.T) {
	tc := testContext{
		context.Background(),
		gomock.NewController(t),
		internal.NewConfigurationWithDefaults(t),
	}

	tc.config.MustSet(config.CodeTestNumbers, []string{"test_phone_number"})

	tests := []struct {
		name    string
		service code.AuthenticationService
		flow    code.Flow
		phone   string
		wantErr bool
	}{
		{"error if flow is not active",
			tc.NewCodeAuthenticationService(
				tc.repoNoCalls(),
				tc.courierNoCalls(),
				clock.NewMock(),
				tc.codeGenerator("0000"),
			),
			tc.invalidFlow(),
			"000000",
			true,
		},
		{"do not send code to test phone number",
			tc.NewCodeAuthenticationService(
				tc.repoNoCodeCreateCode(),
				tc.courierNoCalls(),
				clock.NewMock(),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"test_phone_number",
			false,
		},
		{"send code when not sent before",
			tc.NewCodeAuthenticationService(
				tc.repoNoCodeCreateCode(),
				tc.courier(),
				clock.NewMock(),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"1234",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.service.SendCode(tc.context, tt.flow, tt.phone); (err != nil) != tt.wantErr {
				t.Errorf("SendCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthenticationService_VerifyCode(t *testing.T) {
	tc := testContext{
		context.Background(),
		gomock.NewController(t),
		internal.NewConfigurationWithDefaults(t),
	}

	tc.config.MustSet(config.CodeMaxAttempts, 5)

	tests := []struct {
		name    string
		service code.AuthenticationService
		flow    code.Flow
		code    string
		wantErr bool
	}{
		{"error if flow is not active",
			tc.NewCodeAuthenticationService(
				tc.repoNoCalls(),
				tc.courierNoCalls(),
				clock.NewMock(),
				tc.codeGenerator("0000"),
			),
			tc.invalidFlow(),
			"0000",
			true,
		},
		{"code not found or expired",
			tc.NewCodeAuthenticationService(
				tc.repoNoCode(),
				tc.courierNoCalls(),
				clock.NewMock(),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"1234",
			true,
		},
		{"max number of attempts exceeded",
			tc.NewCodeAuthenticationService(
				tc.repoActiveCodeVerify("0000", newTime("2021-07-10T12:00:00Z"), 5),
				tc.courierNoCalls(),
				fixClock("2021-07-10T12:00:00Z"),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"0000",
			true,
		},
		{"code didn't match",
			tc.NewCodeAuthenticationService(
				tc.repoActiveCodeVerify("0000", newTime("2021-07-10T12:00:00Z"), 0),
				tc.courierNoCalls(),
				fixClock("2021-07-10T12:00:00Z"),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"1234",
			true,
		},
		{"code match",
			tc.NewCodeAuthenticationService(
				tc.repoActiveCodeVerify("0000", newTime("2021-07-10T12:00:00Z"), 0),
				tc.courierNoCalls(),
				fixClock("2021-07-10T12:00:00Z"),
				tc.codeGenerator("0000"),
			),
			tc.validFlowGetID(),
			"0000",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.service.VerifyCode(tc.context, tt.flow, tt.code); (err != nil) != tt.wantErr {
				t.Errorf("VerifyCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func newTime(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		log.Fatal(err)
	}
	return tm
}

func fixClock(t string) clock.Clock {
	c := clock.NewMock()
	c.Set(newTime(t))
	return c
}

func (tc *testContext) invalidFlow() code.Flow {
	m := codeMock.NewMockFlow(tc.controller)
	m.EXPECT().Valid().Return(errors.WithStack(flow.NewFlowExpiredError(time.Now())))
	return m
}

func (tc *testContext) validFlowGetID() code.Flow {
	m := codeMock.NewMockFlow(tc.controller)
	m.EXPECT().Valid().Return(nil)
	m.EXPECT().GetID().MinTimes(1).Return(uuid.FromStringOrNil("00000000-0000-0000-0000-000000000001"))
	return m
}

func (tc *testContext) repoNoCode() code.CodePersister {
	m := codeMock.NewMockCodePersister(tc.controller)
	m.EXPECT().FindActiveCode(tc.context, gomock.Any(), gomock.Any()).Return(nil, nil)
	return m
}

func (tc *testContext) repoNoCodeCreateCode() code.CodePersister {
	m := codeMock.NewMockCodePersister(tc.controller)
	m.EXPECT().DeleteCodes(tc.context, gomock.Any())
	m.EXPECT().CreateCode(tc.context, gomock.Any())
	return m
}

func (tc *testContext) repoActiveCodeSend(authCode string, t time.Time, attempts int) code.CodePersister {
	m := codeMock.NewMockCodePersister(tc.controller)
	m.EXPECT().DeleteCodes(tc.context, gomock.Any())
	m.EXPECT().FindActiveCode(tc.context, gomock.Any(), t).Return(
		&code.Code{
			Identifier: "11111",
			Code:       authCode,
			Attempts:   attempts,
		},
		nil,
	)
	return m
}

func (tc *testContext) repoActiveCodeVerify(authCode string, t time.Time, attempts int) code.CodePersister {
	m := codeMock.NewMockCodePersister(tc.controller)
	m.EXPECT().FindActiveCode(tc.context, gomock.Any(), t).Return(
		&code.Code{
			Identifier: "11111",
			Code:       authCode,
			Attempts:   attempts,
		},
		nil,
	)
	return m
}

func (tc *testContext) repoNoCalls() code.CodePersister {
	m := codeMock.NewMockCodePersister(tc.controller)
	return m
}

func (tc *testContext) courierNoCalls() courier.Courier {
	return courierMock.NewMockCourier(tc.controller)
}

func (tc *testContext) courier() courier.Courier {
	m := courierMock.NewMockCourier(tc.controller)
	m.EXPECT().QueueSMS(tc.context, gomock.Any())
	return m
}

func (tc *testContext) lifespan(s string) *config.Config {
	tc.config.MustSet(config.CodeLifespan, s)
	return tc.config
}

type randomCodeGeneratorStub struct {
	code string
}

//goland:noinspection GoUnusedParameter
func (s *randomCodeGeneratorStub) Generate(max int) string {
	return s.code
}

func (tc *testContext) NewCodeAuthenticationService(
	codePersister code.CodePersister,
	courier courier.Courier,
	clock clock.Clock,
	randomCodeGenerator code.RandomCodeGenerator,
) code.AuthenticationService {

	return code.NewCodeAuthenticationService(&dependencies{
		tc.config,
		codePersister,
		courier,
		clock,
		randomCodeGenerator,
	})
}

func (tc *testContext) codeGenerator(code string) code.RandomCodeGenerator {
	return &randomCodeGeneratorStub{code: code}
}

type dependencies struct {
	config              *config.Config
	codePersister       code.CodePersister
	courier             courier.Courier
	clock               clock.Clock
	randomCodeGenerator code.RandomCodeGenerator
}

func (d *dependencies) Clock() clock.Clock {
	return d.clock
}

func (d *dependencies) CodePersister() code.CodePersister {
	return d.codePersister
}

//goland:noinspection GoUnusedParameter
func (d *dependencies) Courier(ctx context.Context) courier.Courier { return d.courier }

//goland:noinspection GoUnusedParameter
func (d *dependencies) Config(ctx context.Context) *config.Config { return d.config }

func (d *dependencies) RandomCodeGenerator() code.RandomCodeGenerator {
	return d.randomCodeGenerator
}

func (d *dependencies) CourierConfig(ctx context.Context) config.CourierConfigs { return d.config }

func (d *dependencies) HTTPClient(ctx context.Context, opts ...httpx.ResilientOptions) *retryablehttp.Client {
	return httpx.NewResilientClient()
}
