package api

type SandboxConnectionInfo struct {
	Sandboxable    bool
	Address        string
	PublicHostKey  string        `json:"public_host_key"`
	PrivateUserKey string        `json:"private_user_key"`
	Polling        PollingResult `json:"polling"`
	FailureReason  string        `json:"failure_reason,omitempty"`
}

type SandboxConnectionInfoError struct {
	Error string
}
