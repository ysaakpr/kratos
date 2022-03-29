package code

// submitSelfServiceLoginFlowWithPasswordMethod is used to decode the login form payload.
//
// swagger:model submitSelfServiceLoginFlowWithCodeMethod
type submitSelfServiceLoginFlowWithCodeMethod struct {
	// Method should be set to "code" when logging in using the code strategy.
	Method string `json:"method"`

	// Sending the anti-csrf token is only required for browser login flows.
	CSRFToken string `json:"csrf_token"`

	// The user's phone number.
	Identifier string `json:"identifier"`

	// One-time code.
	Code string `json:"code"`
}
