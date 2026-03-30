package api

import (
	"encoding/json"

	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/versions"
)

type Config struct {
	Host                 string
	AccessToken          string
	AccessTokenBackend   accesstoken.Backend
	VersionsBackend      versions.Backend
	SkillVersionsBackend versions.Backend
}

func (c Config) Validate() error {
	if c.Host == "" {
		return errors.New("missing host")
	}

	return nil
}

type InitiateRunConfig struct {
	InitializationParameters []InitializationParameter `json:"initialization_parameters"`
	TaskDefinitions          []RwxDirectoryEntry       `json:"task_definitions"`
	RwxDirectory             []RwxDirectoryEntry       `json:"mint_directory"`
	TargetedTaskKeys         []string                  `json:"targeted_task_keys,omitempty"`
	Title                    string                    `json:"title,omitempty"`
	UseCache                 bool                      `json:"use_cache"`
	Git                      GitMetadata               `json:"git"`
	Patch                    PatchMetadata             `json:"patch"`
	CliState                 string                    `json:"cli_state,omitempty"`
}

type InitializationParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type GitMetadata struct {
	Branch    string `json:"branch,omitempty"`
	Sha       string `json:"sha,omitempty"`
	OriginUrl string `json:"origin_url,omitempty"`
}

type PatchMetadata struct {
	Sent           bool     `json:"sent"`
	LFSFiles       []string `json:"lfs_files"`
	LFSCount       int      `json:"lfs_count"`
	UntrackedFiles []string `json:"untracked_files"`
	UntrackedCount int      `json:"untracked_count"`
	ErrorMessage   string   `json:"cli_error_message,omitempty"`
	GitDirectory   bool     `json:"git_directory"`
	GitInstalled   bool     `json:"git_installed"`
}

type InitiateRunResult struct {
	RunID            string
	RunURL           string
	TargetedTaskKeys []string
	DefinitionPath   string
	Message          string
}

func (c InitiateRunConfig) Validate() error {
	if len(c.TaskDefinitions) == 0 {
		return errors.New("no task definitions")
	}

	return nil
}

type InitiateDispatchConfig struct {
	DispatchKey string            `json:"key"`
	Params      map[string]string `json:"params"`
	Title       string            `json:"title,omitempty"`
	Ref         string            `json:"ref,omitempty"`
}

type InitiateDispatchResult struct {
	DispatchId string
}

func (c InitiateDispatchConfig) Validate() error {
	if c.DispatchKey == "" {
		return errors.New("no dispatch key was provided")
	}

	return nil
}

type GetDispatchConfig struct {
	DispatchId string
}

type GetDispatchRun = struct {
	RunID  string `json:"run_id"`
	RunUrl string `json:"run_url"`
}

type GetDispatchResult struct {
	Status string
	Error  string
	Runs   []GetDispatchRun
}

type ObtainAuthCodeConfig struct {
	Code ObtainAuthCodeCode `json:"code"`
}

type ObtainAuthCodeCode struct {
	DeviceName string `json:"device_name"`
}

type ObtainAuthCodeResult struct {
	AuthorizationUrl string `json:"authorization_url"`
	TokenUrl         string `json:"token_url"`
}

func (c ObtainAuthCodeConfig) Validate() error {
	if c.Code.DeviceName == "" {
		return errors.New("device name must be provided")
	}

	return nil
}

type AcquireTokenResult struct {
	State string `json:"state"` // consumed, expired, authorized, pending
	Token string `json:"token,omitempty"`
}

type WhoamiResult struct {
	OrganizationSlug string  `json:"organization_slug"`
	TokenKind        string  `json:"token_kind"` // organization_access_token, personal_access_token
	UserEmail        *string `json:"user_email,omitempty"`
}

type DocsTokenResult struct {
	Token string `json:"token"`
}

type SetSecretsInVaultConfig struct {
	Secrets   []Secret `json:"secrets"`
	VaultName string   `json:"vault_name"`
}

type Secret struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

type SetSecretsInVaultResult struct {
	SetSecrets []string `json:"set_secrets"`
}

type CreateVaultConfig struct {
	Name                  string                      `json:"name"`
	Unlocked              bool                        `json:"unlocked"`
	RepositoryPermissions []CreateVaultRepoPermission `json:"repository_permissions"`
}

type CreateVaultRepoPermission struct {
	RepositorySlug string `json:"repository_slug"`
	BranchPattern  string `json:"branch_pattern"`
}

type CreateVaultResult struct{}

type DeleteSecretConfig struct {
	SecretName string
	VaultName  string
}

type DeleteSecretResult struct{}

type SetVarConfig struct {
	VaultName string `json:"vault_name"`
	Var       Var    `json:"var"`
}

type Var struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SetVarResult struct{}

type ShowVarConfig struct {
	VarName   string
	VaultName string
}

type ShowVarResult struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DeleteVarConfig struct {
	VarName   string
	VaultName string
}

type DeleteVarResult struct{}

type CreateVaultOidcTokenConfig struct {
	VaultName string `json:"vault_name"`
	Name      string `json:"name,omitempty"`
	Audience  string `json:"audience,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

type CreateVaultOidcTokenResult struct {
	Audience         string `json:"audience"`
	Subject          string `json:"subject"`
	Expression       string `json:"expression"`
	DocumentationURL string `json:"documentation_url"`
}

type ApiPackageInfo struct {
	Description string `json:"description"`
}

type PackageVersionsResult struct {
	Renames     map[string]string            `json:"renames"`
	LatestMajor map[string]string            `json:"latest_major"`
	LatestMinor map[string]map[string]string `json:"latest_minor"`
	Packages    map[string]ApiPackageInfo    `json:"packages"`
}

type PackageDocumentationParameter struct {
	Name        string           `json:"name"`
	Required    bool             `json:"required"`
	Default     *json.RawMessage `json:"default"`
	Description string           `json:"description"`
}

type PackageDocumentationResult struct {
	Name            string                          `json:"name"`
	Version         string                          `json:"version"`
	Readme          string                          `json:"readme"`
	Description     string                          `json:"description"`
	SourceCodeUrl   string                          `json:"source_code_url"`
	IssueTrackerUrl string                          `json:"issue_tracker_url"`
	RenamedTo       *string                         `json:"renamed_to"`
	Parameters      []PackageDocumentationParameter `json:"parameters"`
}

type DefaultBaseResult struct {
	Image  string `json:"image,omitempty"`
	Config string `json:"config,omitempty"`
	Arch   string `json:"arch,omitempty"`
}

type StartImagePushConfig struct {
	TaskID      string                          `json:"task_id"`
	Image       StartImagePushConfigImage       `json:"image"`
	Credentials StartImagePushConfigCredentials `json:"credentials"`
	Compression string                          `json:"compression"`
}

type StartImagePushConfigImage struct {
	Registry   string   `json:"registry"`
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
}

type StartImagePushConfigCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type StartImagePushResult struct {
	PushID string `json:"push_id"`
	RunURL string `json:"run_url"`
}

type ImagePushStatusResult struct {
	Status string `json:"status"`
}

type TaskKeyStatusConfig struct {
	RunID   string
	TaskKey string
}

type TaskIDStatusConfig struct {
	TaskID string
}

const (
	TaskStatusSucceeded = "succeeded"
)

type PollingResult struct {
	Completed bool `json:"completed"`
	BackoffMs *int `json:"backoff_ms,omitempty"`
}

type TaskStatus struct {
	Result string `json:"result"`
}

type TaskStatusResult struct {
	Status  *TaskStatus   `json:"status,omitempty"`
	TaskID  string        `json:"task_id,omitempty"`
	Polling PollingResult `json:"polling"`
}

type LogDownloadRequestResult struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	Filename string `json:"filename"`
	Contents string `json:"contents"`
	RunID    string `json:"run_id"`
}

type ArtifactDownloadRequestResult struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	SizeInBytes int64  `json:"size_in_bytes"`
	Kind        string `json:"kind"`
	Key         string `json:"key"`
}

type RunStatusConfig struct {
	RunID          string
	BranchName     string
	RepositoryName string
	DefinitionPath string
	FailFast       bool
}

type RunStatus struct {
	Result string `json:"result"`
}

type RunStatusResult struct {
	Status     *RunStatus    `json:"run_status,omitempty"`
	TaskStatus *TaskStatus   `json:"task_status,omitempty"`
	RunID      string        `json:"run_id,omitempty"`
	RunURL     string        `json:"run_url,omitempty"`
	Commit     *string       `json:"commit_sha,omitempty"`
	Polling    PollingResult `json:"polling"`
}

type AmbiguousTaskKeyError struct {
	TaskKey string
	Message string
}

func (e *AmbiguousTaskKeyError) Error() string {
	return e.Message
}

func (e *AmbiguousTaskKeyError) Unwrap() error {
	return errors.ErrAmbiguousTaskKey
}

type SandboxInitTemplateResult struct {
	Template string `json:"template"`
}

type SandboxRunSummary struct {
	ID       string `json:"id"`
	RunURL   string `json:"run_url"`
	Title    string `json:"title"`
	CliState string `json:"cli_state"`
}

type ListSandboxRunsResult struct {
	Runs []SandboxRunSummary `json:"runs"`
}

type CreateSandboxTokenConfig struct {
	RunID            string `json:"run_id"`
	ExpiresInSeconds int    `json:"expires_in_seconds,omitempty"`
}

type CreateSandboxTokenResult struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	RunID     string `json:"run_id"`
}
