package cli

import (
	"io"
	"os/exec"

	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/git"
	"github.com/rwx-cloud/rwx/internal/ssh"

	gossh "golang.org/x/crypto/ssh"
)

type APIClient interface {
	GetSkillContent() (string, error)
	GetSkillLatestVersion() (string, error)
	GetDebugConnectionInfo(debugKey string) (api.DebugConnectionInfo, error)
	GetSandboxConnectionInfo(runID, scopedToken string) (api.SandboxConnectionInfo, error)
	CreateSandboxToken(api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error)
	GetDispatch(api.GetDispatchConfig) (*api.GetDispatchResult, error)
	InitiateRun(api.InitiateRunConfig) (*api.InitiateRunResult, error)
	InitiateDispatch(api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error)
	ObtainAuthCode(api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error)
	AcquireToken(tokenUrl string) (*api.AcquireTokenResult, error)
	Whoami() (*api.WhoamiResult, error)
	CreateDocsToken() (*api.DocsTokenResult, error)
	SetSecretsInVault(api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error)
	CreateVault(api.CreateVaultConfig) (*api.CreateVaultResult, error)
	CreateVaultOidcToken(api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error)
	DeleteSecret(api.DeleteSecretConfig) (*api.DeleteSecretResult, error)
	SetVar(api.SetVarConfig) (*api.SetVarResult, error)
	ShowVar(api.ShowVarConfig) (*api.ShowVarResult, error)
	DeleteVar(api.DeleteVarConfig) (*api.DeleteVarResult, error)
	GetPackageVersions() (*api.PackageVersionsResult, error)
	GetPackageDocumentation(packageName string) (*api.PackageDocumentationResult, error)
	GetDefaultBase() (api.DefaultBaseResult, error)
	StartImagePush(cfg api.StartImagePushConfig) (api.StartImagePushResult, error)
	ImagePushStatus(pushID string) (api.ImagePushStatusResult, error)
	TaskKeyStatus(api.TaskKeyStatusConfig) (api.TaskStatusResult, error)
	TaskIDStatus(api.TaskIDStatusConfig) (api.TaskStatusResult, error)
	RunStatus(api.RunStatusConfig) (api.RunStatusResult, error)
	GetLogDownloadRequest(taskId string) (api.LogDownloadRequestResult, error)
	GetLogDownloadRequestByTaskKey(runID, taskKey string) (api.LogDownloadRequestResult, error)
	DownloadLogs(api.LogDownloadRequestResult, ...int) ([]byte, error)
	GetAllArtifactDownloadRequests(taskId string) ([]api.ArtifactDownloadRequestResult, error)
	GetAllArtifactDownloadRequestsByTaskKey(runID, taskKey string) ([]api.ArtifactDownloadRequestResult, error)
	GetArtifactDownloadRequest(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error)
	GetArtifactDownloadRequestByTaskKey(runID, taskKey, artifactKey string) (api.ArtifactDownloadRequestResult, error)
	DownloadArtifact(api.ArtifactDownloadRequestResult) ([]byte, error)
	GetRunPrompt(runID string) (string, error)
	GetRunPromptByTaskKey(runID, taskKey string) (string, error)
	GetSandboxInitTemplate() (api.SandboxInitTemplateResult, error)
	ListSandboxRuns() (*api.ListSandboxRunsResult, error)
	CancelRun(runID, scopedToken string) error
}

var _ APIClient = api.Client{}

type SSHClient interface {
	Close() error
	Connect(addr string, cfg gossh.ClientConfig) error
	InteractiveSession() error
	ExecuteCommand(command string) (int, error)
	ExecuteCommandWithStdin(command string, stdin io.Reader) (int, error)
	ExecuteCommandWithOutput(command string) (int, string, error)
	ExecuteCommandWithStdinAndCombinedOutput(command string, stdin io.Reader) (int, string, error)
}

var _ SSHClient = (*ssh.Client)(nil)

type GitClient interface {
	GetBranch() string
	GetHead() string
	GetCommit() (string, error)
	GetOriginUrl() string
	GeneratePatchFile(destDir string, pathspec []string) (git.PatchFile, error)
	GeneratePatch(pathspec []string) ([]byte, *git.LFSChangedFilesMetadata, error)
	ApplyPatch(patch []byte) *exec.Cmd
	ApplyPatchReject(patch []byte) *exec.Cmd
	IsInstalled() bool
	IsInsideWorkTree() bool
}
