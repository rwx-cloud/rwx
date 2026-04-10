package mocks

import (
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type API struct {
	MockGetSkillContent                         func() (string, error)
	MockGetSkillLatestVersion                   func() (string, error)
	MockInitiateRun                             func(api.InitiateRunConfig) (*api.InitiateRunResult, error)
	MockGetDebugConnectionInfo                  func(runID string) (api.DebugConnectionInfo, error)
	MockGetSandboxConnectionInfo                func(runID, scopedToken string) (api.SandboxConnectionInfo, error)
	MockCreateSandboxToken                      func(api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error)
	MockObtainAuthCode                          func(api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error)
	MockAcquireToken                            func(tokenUrl string) (*api.AcquireTokenResult, error)
	MockWhoami                                  func() (*api.WhoamiResult, error)
	MockCreateDocsToken                         func() (*api.DocsTokenResult, error)
	MockSetSecretsInVault                       func(api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error)
	MockCreateVault                             func(api.CreateVaultConfig) (*api.CreateVaultResult, error)
	MockCreateVaultOidcToken                    func(api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error)
	MockDeleteSecret                            func(api.DeleteSecretConfig) (*api.DeleteSecretResult, error)
	MockSetVar                                  func(api.SetVarConfig) (*api.SetVarResult, error)
	MockShowVar                                 func(api.ShowVarConfig) (*api.ShowVarResult, error)
	MockDeleteVar                               func(api.DeleteVarConfig) (*api.DeleteVarResult, error)
	MockGetPackageVersions                      func() (*api.PackageVersionsResult, error)
	MockGetPackageDocumentation                 func(string) (*api.PackageDocumentationResult, error)
	MockInitiateDispatch                        func(api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error)
	MockGetDispatch                             func(api.GetDispatchConfig) (*api.GetDispatchResult, error)
	MockGetDefaultBase                          func() (api.DefaultBaseResult, error)
	MockMcpGetRunTestFailures                   func(api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error)
	MockStartImagePush                          func(api.StartImagePushConfig) (api.StartImagePushResult, error)
	MockImagePushStatus                         func(string) (api.ImagePushStatusResult, error)
	MockTaskKeyStatus                           func(api.TaskKeyStatusConfig) (api.TaskStatusResult, error)
	MockTaskIDStatus                            func(api.TaskIDStatusConfig) (api.TaskStatusResult, error)
	MockRunStatus                               func(api.RunStatusConfig) (api.RunStatusResult, error)
	MockGetLogDownloadRequest                   func(string) (api.LogDownloadRequestResult, error)
	MockGetLogDownloadRequestByTaskKey          func(string, string) (api.LogDownloadRequestResult, error)
	MockDownloadLogs                            func(api.LogDownloadRequestResult) ([]byte, error)
	MockGetAllArtifactDownloadRequests          func(string) ([]api.ArtifactDownloadRequestResult, error)
	MockGetAllArtifactDownloadRequestsByTaskKey func(string, string) ([]api.ArtifactDownloadRequestResult, error)
	MockGetArtifactDownloadRequest              func(string, string) (api.ArtifactDownloadRequestResult, error)
	MockGetArtifactDownloadRequestByTaskKey     func(string, string, string) (api.ArtifactDownloadRequestResult, error)
	MockDownloadArtifact                        func(api.ArtifactDownloadRequestResult) ([]byte, error)
	MockGetRunPrompt                            func(string) (string, error)
	MockGetRunPromptByTaskKey                   func(string, string) (string, error)
	MockGetSandboxInitTemplate                  func() (api.SandboxInitTemplateResult, error)
	MockListSandboxRuns                         func() (*api.ListSandboxRunsResult, error)
	MockCancelRun                               func(runID, scopedToken string) error
}

func (c *API) GetSkillContent() (string, error) {
	if c.MockGetSkillContent != nil {
		return c.MockGetSkillContent()
	}

	return "", errors.New("MockGetSkillContent was not configured")
}

func (c *API) GetSkillLatestVersion() (string, error) {
	if c.MockGetSkillLatestVersion != nil {
		return c.MockGetSkillLatestVersion()
	}

	return "", errors.New("MockGetSkillLatestVersion was not configured")
}

func (c *API) InitiateRun(cfg api.InitiateRunConfig) (*api.InitiateRunResult, error) {
	if c.MockInitiateRun != nil {
		return c.MockInitiateRun(cfg)
	}

	return nil, errors.New("MockInitiateRun was not configured")
}

func (c *API) GetDebugConnectionInfo(runID string) (api.DebugConnectionInfo, error) {
	if c.MockGetDebugConnectionInfo != nil {
		return c.MockGetDebugConnectionInfo(runID)
	}

	return api.DebugConnectionInfo{}, errors.New("MockGetDebugConnectionInfo was not configured")
}

func (c *API) GetSandboxConnectionInfo(runID, scopedToken string) (api.SandboxConnectionInfo, error) {
	if c.MockGetSandboxConnectionInfo != nil {
		return c.MockGetSandboxConnectionInfo(runID, scopedToken)
	}

	return api.SandboxConnectionInfo{}, errors.New("MockGetSandboxConnectionInfo was not configured")
}

func (c *API) CreateSandboxToken(cfg api.CreateSandboxTokenConfig) (*api.CreateSandboxTokenResult, error) {
	if c.MockCreateSandboxToken != nil {
		return c.MockCreateSandboxToken(cfg)
	}

	return nil, errors.New("MockCreateSandboxToken was not configured")
}

func (c *API) ObtainAuthCode(cfg api.ObtainAuthCodeConfig) (*api.ObtainAuthCodeResult, error) {
	if c.MockObtainAuthCode != nil {
		return c.MockObtainAuthCode(cfg)
	}

	return nil, errors.New("MockObtainAuthCode was not configured")
}

func (c *API) AcquireToken(tokenUrl string) (*api.AcquireTokenResult, error) {
	if c.MockAcquireToken != nil {
		return c.MockAcquireToken(tokenUrl)
	}

	return nil, errors.New("MockAcquireToken was not configured")
}

func (c *API) Whoami() (*api.WhoamiResult, error) {
	if c.MockWhoami != nil {
		return c.MockWhoami()
	}

	return nil, errors.New("MockWhoami was not configured")
}

func (c *API) CreateDocsToken() (*api.DocsTokenResult, error) {
	if c.MockCreateDocsToken != nil {
		return c.MockCreateDocsToken()
	}

	return nil, errors.New("MockCreateDocsToken was not configured")
}

func (c *API) SetSecretsInVault(cfg api.SetSecretsInVaultConfig) (*api.SetSecretsInVaultResult, error) {
	if c.MockSetSecretsInVault != nil {
		return c.MockSetSecretsInVault(cfg)
	}

	return nil, errors.New("MockSetSecretsInVault was not configured")
}

func (c *API) CreateVault(cfg api.CreateVaultConfig) (*api.CreateVaultResult, error) {
	if c.MockCreateVault != nil {
		return c.MockCreateVault(cfg)
	}

	return nil, errors.New("MockCreateVault was not configured")
}

func (c *API) CreateVaultOidcToken(cfg api.CreateVaultOidcTokenConfig) (*api.CreateVaultOidcTokenResult, error) {
	if c.MockCreateVaultOidcToken != nil {
		return c.MockCreateVaultOidcToken(cfg)
	}

	return nil, errors.New("MockCreateVaultOidcToken was not configured")
}

func (c *API) DeleteSecret(cfg api.DeleteSecretConfig) (*api.DeleteSecretResult, error) {
	if c.MockDeleteSecret != nil {
		return c.MockDeleteSecret(cfg)
	}

	return nil, errors.New("MockDeleteSecret was not configured")
}

func (c *API) SetVar(cfg api.SetVarConfig) (*api.SetVarResult, error) {
	if c.MockSetVar != nil {
		return c.MockSetVar(cfg)
	}

	return nil, errors.New("MockSetVar was not configured")
}

func (c *API) ShowVar(cfg api.ShowVarConfig) (*api.ShowVarResult, error) {
	if c.MockShowVar != nil {
		return c.MockShowVar(cfg)
	}

	return nil, errors.New("MockShowVar was not configured")
}

func (c *API) DeleteVar(cfg api.DeleteVarConfig) (*api.DeleteVarResult, error) {
	if c.MockDeleteVar != nil {
		return c.MockDeleteVar(cfg)
	}

	return nil, errors.New("MockDeleteVar was not configured")
}

func (c *API) GetPackageVersions() (*api.PackageVersionsResult, error) {
	if c.MockGetPackageVersions != nil {
		return c.MockGetPackageVersions()
	}

	return nil, errors.New("MockGetPackageVersions was not configured")
}

func (c *API) GetPackageDocumentation(packageName string) (*api.PackageDocumentationResult, error) {
	if c.MockGetPackageDocumentation != nil {
		return c.MockGetPackageDocumentation(packageName)
	}

	return nil, errors.New("MockGetPackageDocumentation was not configured")
}

func (c *API) InitiateDispatch(cfg api.InitiateDispatchConfig) (*api.InitiateDispatchResult, error) {
	if c.MockInitiateDispatch != nil {
		return c.MockInitiateDispatch(cfg)
	}

	return nil, errors.New("MockInitiateDispatch was not configured")
}

func (c *API) GetDispatch(cfg api.GetDispatchConfig) (*api.GetDispatchResult, error) {
	if c.MockGetDispatch != nil {
		return c.MockGetDispatch(cfg)
	}

	return nil, errors.New("MockGetDispatch was not configured")
}

func (c *API) GetDefaultBase() (api.DefaultBaseResult, error) {
	if c.MockGetDefaultBase != nil {
		return c.MockGetDefaultBase()
	}

	return api.DefaultBaseResult{}, errors.New("MockGetDefaultBase was not configured")
}

func (c *API) McpGetRunTestFailures(cfg api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error) {
	if c.MockMcpGetRunTestFailures != nil {
		return c.MockMcpGetRunTestFailures(cfg)
	}

	return nil, errors.New("MockMcpGetRunTestFailures was not configured")
}

func (c *API) StartImagePush(cfg api.StartImagePushConfig) (api.StartImagePushResult, error) {
	if c.MockStartImagePush != nil {
		return c.MockStartImagePush(cfg)
	}

	return api.StartImagePushResult{}, errors.New("MockStartImagePush was not configured")
}

func (c *API) ImagePushStatus(pushID string) (api.ImagePushStatusResult, error) {
	if c.MockImagePushStatus != nil {
		return c.MockImagePushStatus(pushID)
	}

	return api.ImagePushStatusResult{}, errors.New("MockImagePushStatus was not configured")
}

func (c *API) TaskKeyStatus(cfg api.TaskKeyStatusConfig) (api.TaskStatusResult, error) {
	if c.MockTaskKeyStatus != nil {
		return c.MockTaskKeyStatus(cfg)
	}

	return api.TaskStatusResult{}, errors.New("MockTaskKeyStatus was not configured")
}

func (c *API) TaskIDStatus(cfg api.TaskIDStatusConfig) (api.TaskStatusResult, error) {
	if c.MockTaskIDStatus != nil {
		return c.MockTaskIDStatus(cfg)
	}

	return api.TaskStatusResult{}, errors.New("MockTaskIDStatus was not configured")
}

func (c *API) RunStatus(cfg api.RunStatusConfig) (api.RunStatusResult, error) {
	if c.MockRunStatus != nil {
		return c.MockRunStatus(cfg)
	}

	return api.RunStatusResult{}, errors.New("MockRunStatus was not configured")
}

func (c *API) GetLogDownloadRequest(taskId string) (api.LogDownloadRequestResult, error) {
	if c.MockGetLogDownloadRequest != nil {
		return c.MockGetLogDownloadRequest(taskId)
	}

	return api.LogDownloadRequestResult{}, errors.New("MockGetLogDownloadRequest was not configured")
}

func (c *API) GetLogDownloadRequestByTaskKey(runID, taskKey string) (api.LogDownloadRequestResult, error) {
	if c.MockGetLogDownloadRequestByTaskKey != nil {
		return c.MockGetLogDownloadRequestByTaskKey(runID, taskKey)
	}
	return api.LogDownloadRequestResult{}, errors.New("MockGetLogDownloadRequestByTaskKey was not configured")
}

func (c *API) DownloadLogs(request api.LogDownloadRequestResult, maxRetryDurationSeconds ...int) ([]byte, error) {
	if c.MockDownloadLogs != nil {
		return c.MockDownloadLogs(request)
	}

	return nil, errors.New("MockDownloadLogs was not configured")
}

func (c *API) GetAllArtifactDownloadRequests(taskId string) ([]api.ArtifactDownloadRequestResult, error) {
	if c.MockGetAllArtifactDownloadRequests != nil {
		return c.MockGetAllArtifactDownloadRequests(taskId)
	}

	return nil, errors.New("MockGetAllArtifactDownloadRequests was not configured")
}

func (c *API) GetAllArtifactDownloadRequestsByTaskKey(runID, taskKey string) ([]api.ArtifactDownloadRequestResult, error) {
	if c.MockGetAllArtifactDownloadRequestsByTaskKey != nil {
		return c.MockGetAllArtifactDownloadRequestsByTaskKey(runID, taskKey)
	}
	return nil, errors.New("MockGetAllArtifactDownloadRequestsByTaskKey was not configured")
}

func (c *API) GetArtifactDownloadRequest(taskId, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
	if c.MockGetArtifactDownloadRequest != nil {
		return c.MockGetArtifactDownloadRequest(taskId, artifactKey)
	}

	return api.ArtifactDownloadRequestResult{}, errors.New("MockGetArtifactDownloadRequest was not configured")
}

func (c *API) GetArtifactDownloadRequestByTaskKey(runID, taskKey, artifactKey string) (api.ArtifactDownloadRequestResult, error) {
	if c.MockGetArtifactDownloadRequestByTaskKey != nil {
		return c.MockGetArtifactDownloadRequestByTaskKey(runID, taskKey, artifactKey)
	}
	return api.ArtifactDownloadRequestResult{}, errors.New("MockGetArtifactDownloadRequestByTaskKey was not configured")
}

func (c *API) DownloadArtifact(request api.ArtifactDownloadRequestResult) ([]byte, error) {
	if c.MockDownloadArtifact != nil {
		return c.MockDownloadArtifact(request)
	}

	return nil, errors.New("MockDownloadArtifact was not configured")
}

func (c *API) GetRunPrompt(runID string) (string, error) {
	if c.MockGetRunPrompt != nil {
		return c.MockGetRunPrompt(runID)
	}

	return "", errors.New("MockGetRunPrompt was not configured")
}

func (c *API) GetRunPromptByTaskKey(runID, taskKey string) (string, error) {
	if c.MockGetRunPromptByTaskKey != nil {
		return c.MockGetRunPromptByTaskKey(runID, taskKey)
	}

	return "", errors.New("MockGetRunPromptByTaskKey was not configured")
}

func (c *API) ListSandboxRuns() (*api.ListSandboxRunsResult, error) {
	if c.MockListSandboxRuns != nil {
		return c.MockListSandboxRuns()
	}

	return nil, errors.New("MockListSandboxRuns was not configured")
}

func (c *API) CancelRun(runID, scopedToken string) error {
	if c.MockCancelRun != nil {
		return c.MockCancelRun(runID, scopedToken)
	}

	return errors.New("MockCancelRun was not configured")
}

func (c *API) GetSandboxInitTemplate() (api.SandboxInitTemplateResult, error) {
	if c.MockGetSandboxInitTemplate != nil {
		return c.MockGetSandboxInitTemplate()
	}

	return api.SandboxInitTemplateResult{}, errors.New("MockGetSandboxInitTemplate was not configured")
}
