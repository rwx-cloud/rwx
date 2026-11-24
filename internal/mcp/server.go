package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rwx-cloud/rwx/internal/accesstoken"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/versions"
)

const DeprecationNotice = "The local RWX MCP server (rwx mcp serve) is deprecated. Hosted MCP setup: https://www.rwx.com/docs/mcp."

type ServeConfig struct {
	AccessToken        string
	Host               string
	AccessTokenBackend accesstoken.Backend
}

func Serve(ctx context.Context, config ServeConfig) error {
	apiClient, err := api.NewClient(api.Config{
		AccessToken:        config.AccessToken,
		Host:               config.Host,
		AccessTokenBackend: config.AccessTokenBackend,
	})
	if err != nil {
		return err
	}

	server := NewServer(ServerConfig{APIClient: apiClient})
	return server.Run(ctx, &mcp.StdioTransport{})
}

type Server struct {
	ms  *mcp.Server
	api APIClient
}

type ServerConfig struct {
	APIClient APIClient
}

func NewServer(config ServerConfig) *Server {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "rwx-cli-mcp-server",
		Version: versions.GetCliCurrentVersion().String(),
	}, &mcp.ServerOptions{})

	server := &Server{ms: mcpServer, api: config.APIClient}
	server.addTools()

	return server
}

func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	return s.ms.Run(ctx, transport)
}

func (s *Server) addTools() {
	mcp.AddTool(s.ms, &mcp.Tool{
		Name: "get_run_test_failures",
		Description: DeprecationNotice + `

Get the list of failed tests for the given run(s).

Run URLs or IDs are for RWX (not other CI providers) and can be found from:
- RWX URLs like: https://cloud.rwx.com/mint/my-org/runs/90bf40a00be843ed89cc1d9c8535f0ca
- Run IDs like: 90bf40a00be843ed89cc1d9c8535f0ca
- GitHub PR status checks: ` + "`" + `gh pr view <pr> --json statusCheckRollup` + "`" + ` (extract from targetUrl)
- GitHub commit status (when not a PR): ` + "`" + `gh api repos/:owner/:repo/commits/<sha>/status` + "`" + `

When finding runs, typically target runs for a specific commit or PR.

Examples:
- Full URLs: ["https://cloud.rwx.com/mint/my-org/runs/90bf40a00be843ed89cc1d9c8535f0ca"]
- Just IDs: ["90bf40a00be843ed89cc1d9c8535f0ca", "41fcc0f38a054716b83c49cd7662539a"]`,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: true,
		},
	}, s.getRunTestFailures)
}

type GetRunFailedTestsInput struct {
	RunUrls []string `json:"run_urls" jsonschema:"The URLs or IDs of the run"`
}

func (s *Server) getRunTestFailures(ctx context.Context, request *mcp.CallToolRequest, input GetRunFailedTestsInput) (*mcp.CallToolResult, any, error) {
	res, err := s.api.McpGetRunTestFailures(api.McpGetRunTestFailuresRequest{RunUrls: input.RunUrls})
	if err != nil {
		return nil, nil, err
	}

	return mcpToolTextResult(res.Text), nil, nil
}

func mcpToolTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}
}
