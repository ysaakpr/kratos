package code_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofrs/uuid"
	kratos "github.com/ory/kratos-client-go"
	"github.com/ory/kratos/courier/template/sms"
	"github.com/ory/kratos/driver/config"
	"github.com/ory/kratos/identity"
	"github.com/ory/kratos/internal"
	"github.com/ory/kratos/internal/testhelpers"
	"github.com/ory/kratos/selfservice/flow"
	"github.com/ory/kratos/selfservice/flow/registration"
	"github.com/ory/kratos/selfservice/strategy/code"
	"github.com/ory/kratos/x"
	"github.com/ory/x/snapshotx"
	"github.com/ory/x/urlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRegistration(t *testing.T) {
	t.Run("case=registration", func(t *testing.T) {
		conf, reg := internal.NewFastRegistryWithMocks(t)

		router := x.NewRouterPublic()
		admin := x.NewRouterAdmin()
		conf.MustSet(config.ViperKeySelfServiceStrategyConfig+"."+string(identity.CredentialsTypeCode), map[string]interface{}{"enabled": true})

		publicTS, _ := testhelpers.NewKratosServerWithRouters(t, reg, router, admin)
		//errTS := testhelpers.NewErrorTestServer(t, reg)
		//uiTS := testhelpers.NewRegistrationUIFlowEchoServer(t, reg)
		redirTS := testhelpers.NewRedirSessionEchoTS(t, reg)

		// Overwrite these two to ensure that they run
		conf.MustSet(config.ViperKeySelfServiceBrowserDefaultReturnTo, redirTS.URL+"/default-return-to")
		conf.MustSet(config.ViperKeySelfServiceRegistrationAfter+"."+config.DefaultBrowserReturnURL, redirTS.URL+"/registration-return-ts")
		testhelpers.SetDefaultIdentitySchema(conf, "file://./stub/default.schema.json")
		conf.MustSet(config.CodeMaxAttempts, 5)

		t.Run("case=should fail if identifier changed when submitted with code", func(t *testing.T) {
			identifier := "+11111111111"

			_, err := reg.CourierPersister().NextMessages(context.Background(), 10)
			assert.Error(t, err, "Courier queue should be empty.")

			hc := new(http.Client)

			f := testhelpers.InitializeRegistrationFlow(t, true, hc, publicTS, false)

			var values = func(v url.Values) {
				v.Set("method", "code")
				v.Set("traits.phone", identifier)
			}
			testhelpers.SubmitRegistrationFormWithFlow(t, true, hc, values,
				false, http.StatusBadRequest, publicTS.URL+registration.RouteSubmitFlow, f)

			messages, err := reg.CourierPersister().NextMessages(context.Background(), 10)
			assert.NoError(t, err, "Courier queue should not be empty.")
			assert.Equal(t, 1, len(messages))
			var smsModel sms.CodeMessageModel
			err = json.Unmarshal(messages[0].TemplateData, &smsModel)
			assert.NoError(t, err)

			values = func(v url.Values) {
				v.Set("method", "code")
				v.Set("traits.phone", identifier+"2")
				v.Set("code", smsModel.Code)
			}

			body := testhelpers.SubmitRegistrationFormWithFlow(t, true, hc, values,
				false, http.StatusBadRequest, publicTS.URL+registration.RouteSubmitFlow, f)
			assert.Contains(t, body, "not equal to verified phone")

		})

		var expectSuccessfulLogin = func(
			t *testing.T, isAPI, isSPA bool, hc *http.Client,
			expectReturnTo string,
			identifier string,
		) string {
			if hc == nil {
				if isAPI {
					hc = new(http.Client)
				} else {
					hc = testhelpers.NewClientWithCookies(t)
				}
			}

			_, err := reg.CourierPersister().NextMessages(context.Background(), 10)
			assert.Error(t, err, "Courier queue should be empty.")

			f := testhelpers.InitializeRegistrationFlow(t, isAPI, hc, publicTS, isSPA)

			assert.Empty(t, getRegistrationNode(f, "code"))
			assert.NotEmpty(t, getRegistrationNode(f, "traits.phone"))

			var values = func(v url.Values) {
				v.Set("method", "code")
				v.Set("traits.phone", identifier)
			}
			body := testhelpers.SubmitRegistrationFormWithFlow(t, isAPI, hc, values,
				isSPA, http.StatusBadRequest, expectReturnTo, f)

			messages, err := reg.CourierPersister().NextMessages(context.Background(), 10)
			assert.NoError(t, err, "Courier queue should not be empty.")
			assert.Equal(t, 1, len(messages))
			var smsModel sms.CodeMessageModel
			err = json.Unmarshal(messages[0].TemplateData, &smsModel)
			assert.NoError(t, err)

			st := gjson.Get(body, "session_token").String()
			assert.Empty(t, st, "Response body: %s", body) //No session token as we have not presented the code yet

			values = func(v url.Values) {
				v.Set("method", "code")
				v.Set("traits.phone", identifier)
				v.Set("code", smsModel.Code)
			}

			body = testhelpers.SubmitRegistrationFormWithFlow(t, isAPI, hc, values,
				isSPA, http.StatusOK, expectReturnTo, f)

			assert.Equal(t, identifier, gjson.Get(body, "session.identity.traits.phone").String(),
				"%s", body)
			identityID, err := uuid.FromString(gjson.Get(body, "identity.id").String())
			assert.NoError(t, err)
			i, err := reg.PrivilegedIdentityPool().GetIdentityConfidential(context.Background(), identityID)
			assert.NoError(t, err)
			assert.NotEmpty(t, i.Credentials, "%s", body)
			assert.Equal(t, identifier, i.Credentials["code"].Identifiers[0], "%s", body)
			assert.NotEmpty(t, gjson.Get(body, "session_token").String(), "%s", body)
			assert.Equal(t, identifier, gjson.Get(body, "identity.verifiable_addresses.0.value").String())
			assert.Equal(t, "true", gjson.Get(body, "identity.verifiable_addresses.0.verified").String())

			return body
		}

		t.Run("case=should pass and set up a session", func(t *testing.T) {
			testhelpers.SetDefaultIdentitySchema(conf, "file://./stub/default.schema.json")
			conf.MustSet(config.HookStrategyKey(config.ViperKeySelfServiceRegistrationAfter, identity.CredentialsTypeCode.String()), []config.SelfServiceHook{{Name: "session"}})
			t.Cleanup(func() {
				conf.MustSet(config.HookStrategyKey(config.ViperKeySelfServiceRegistrationAfter, identity.CredentialsTypeCode.String()), nil)
			})

			identifier := "+11111111111"

			t.Run("type=api", func(t *testing.T) {
				expectSuccessfulLogin(t, true, false, nil,
					publicTS.URL+registration.RouteSubmitFlow, identifier)
			})

			//t.Run("type=spa", func(t *testing.T) {
			//	hc := testhelpers.NewClientWithCookies(t)
			//	body := expectSuccessfulLogin(t, false, true, hc, func(v url.Values) {
			//		v.Set("traits.username", "registration-identifier-8-spa")
			//		v.Set("password", x.NewUUID().String())
			//		v.Set("traits.foobar", "bar")
			//	})
			//	assert.Equal(t, `registration-identifier-8-spa`, gjson.Get(body, "identity.traits.username").String(), "%s", body)
			//	assert.Empty(t, gjson.Get(body, "session_token").String(), "%s", body)
			//	assert.NotEmpty(t, gjson.Get(body, "session.id").String(), "%s", body)
			//})
			//
			//t.Run("type=browser", func(t *testing.T) {
			//	body := expectSuccessfulLogin(t, false, false, nil, func(v url.Values) {
			//		v.Set("traits.username", "registration-identifier-8-browser")
			//		v.Set("password", x.NewUUID().String())
			//		v.Set("traits.foobar", "bar")
			//	})
			//	assert.Equal(t, `registration-identifier-8-browser`, gjson.Get(body, "identity.traits.username").String(), "%s", body)
			//})
		})

		t.Run("case=should create verifiable address", func(t *testing.T) {
			identifier := "+1234567890"
			createdIdentity := &identity.Identity{
				SchemaID: "default",
				Traits:   identity.Traits(fmt.Sprintf(`{"phone":"%s"}`, identifier)),
				State:    identity.StateActive}
			err := reg.IdentityManager().Create(context.Background(), createdIdentity)
			assert.NoError(t, err)

			i, err := reg.PrivilegedIdentityPool().GetIdentityConfidential(context.Background(), createdIdentity.ID)
			assert.NoError(t, err)
			assert.Equal(t, identifier, i.VerifiableAddresses[0].Value)
			assert.False(t, i.VerifiableAddresses[0].Verified)
			assert.Equal(t, identity.VerifiableAddressStatusPending, i.VerifiableAddresses[0].Status)
		})

		t.Run("method=TestPopulateSignUpMethod", func(t *testing.T) {
			conf.MustSet(config.ViperKeyPublicBaseURL, "https://foo/")

			sr, err := registration.NewFlow(conf, time.Minute, "nosurf", &http.Request{URL: urlx.ParseOrPanic("/")}, flow.TypeBrowser)
			require.NoError(t, err)
			require.NoError(t, reg.RegistrationStrategies(context.Background()).
				MustStrategy(identity.CredentialsTypeCode).(*code.Strategy).PopulateRegistrationMethod(&http.Request{}, sr))

			snapshotx.SnapshotTExcept(t, sr.UI, []string{"action", "nodes.0.attributes.value"})
		})

	})
}

func getRegistrationNode(f *kratos.SelfServiceRegistrationFlow, nodeName string) *kratos.UiNode {
	for _, n := range f.Ui.Nodes {
		if n.Attributes.UiNodeInputAttributes.Name == nodeName {
			return &n
		}
	}
	return nil
}
