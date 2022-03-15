package sms

import (
	"context"
	"encoding/json"
	"github.com/ory/kratos/courier/template"
	"os"
)

type (
	CodeMessage struct {
		d template.Dependencies
		m *CodeMessageModel
	}
	CodeMessageModel struct {
		To   string
		Code string
	}
)

func NewCodeMessage(d template.Dependencies, m *CodeMessageModel) *CodeMessage {
	return &CodeMessage{d: d, m: m}
}

func (t *CodeMessage) PhoneNumber() (string, error) {
	return t.m.To, nil
}

func (t *CodeMessage) SMSBody(ctx context.Context) (string, error) {
	return template.LoadText(ctx, t.d, os.DirFS(t.d.CourierConfig(ctx).CourierTemplatesRoot()), "login/sms.body.gotmpl", "login/sms.body*", t.m, t.d.CourierConfig(ctx).CourierTemplatesVerificationValidSMS())
}

func (t *CodeMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.m)
}
