package mcp_test

import (
	"context"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rwx-cloud/rwx/internal/api"
	"github.com/rwx-cloud/rwx/internal/errors"
	"github.com/rwx-cloud/rwx/internal/mcp"
	"github.com/rwx-cloud/rwx/internal/mocks"
	"github.com/stretchr/testify/require"
)

var _ mcp.APIClient = (*mocks.API)(nil)

type mcpTestSetup struct {
	ctx     context.Context
	client  *gomcp.Client
	session *gomcp.ClientSession
	server  *mcp.Server
}

func (m *mcpTestSetup) cleanup() {
	if m.session != nil {
		m.session.Close()
	}
}

// setupMCPTest creates a complete end-to-end MCP test environment with client-server communication
func setupMCPTest(t *testing.T, mockAPI *mocks.API) *mcpTestSetup {
	t.Helper()

	ctx := context.Background()
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()

	server := mcp.NewServer(mcp.ServerConfig{APIClient: mockAPI})
	go func() {
		if err := server.Run(ctx, serverTransport); err != nil {
			t.Logf("Server error: %v", err)
		}
	}()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)

	return &mcpTestSetup{
		ctx:     ctx,
		client:  client,
		session: session,
		server:  server,
	}
}

// expectTextResult validates a successful text result
func expectTextResult(t *testing.T, result *gomcp.CallToolResult, expectedText string) {
	t.Helper()

	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	textContent, ok := result.Content[0].(*gomcp.TextContent)
	require.True(t, ok)
	require.Equal(t, expectedText, textContent.Text)
}

// expectErrorResult validates an error result
func expectErrorResult(t *testing.T, result *gomcp.CallToolResult, expectedErrorText string) {
	t.Helper()

	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].(*gomcp.TextContent).Text, expectedErrorText)
}

func TestNewServer(t *testing.T) {
	t.Run("creates server with correct configuration", func(t *testing.T) {
		mockAPI := &mocks.API{}
		config := mcp.ServerConfig{APIClient: mockAPI}

		server := mcp.NewServer(config)

		require.NotNil(t, server)
	})
}

func TestServer_ListTools(t *testing.T) {
	setup := setupMCPTest(t, &mocks.API{})
	defer setup.cleanup()

	result, err := setup.session.ListTools(setup.ctx, &gomcp.ListToolsParams{})
	require.NoError(t, err)
	require.Len(t, result.Tools, 1)
	require.Equal(t, "get_run_test_failures", result.Tools[0].Name)
	require.Contains(t, result.Tools[0].Description, "The local RWX MCP server (rwx mcp serve) is deprecated and continues to work during migration.")
	require.Contains(t, result.Tools[0].Description, "https://www.rwx.com/docs/mcp")
	require.Contains(t, result.Tools[0].Description, "Get the list of failed tests for the given run(s).")
}

func TestServer_GetRunTestFailures(t *testing.T) {
	t.Run("successfully retrieves test failures", func(t *testing.T) {
		expectedText := "Failed tests:\n- spec/models/user_spec.rb:42\n- spec/controllers/home_controller_spec.rb:15"

		mockAPI := &mocks.API{
			MockMcpGetRunTestFailures: func(req api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error) {
				require.Equal(t, []string{"https://cloud.rwx.com/mint/org/runs/123"}, req.RunUrls)
				return &api.McpTextResult{Type: "text", Text: expectedText}, nil
			},
		}

		setup := setupMCPTest(t, mockAPI)
		defer setup.cleanup()

		result, err := setup.session.CallTool(setup.ctx, &gomcp.CallToolParams{
			Name: "get_run_test_failures",
			Arguments: map[string]any{
				"run_urls": []string{"https://cloud.rwx.com/mint/org/runs/123"},
			},
		})
		require.NoError(t, err)
		expectTextResult(t, result, expectedText)
	})

	t.Run("handles multiple run URLs", func(t *testing.T) {
		expectedRunUrls := []string{
			"https://cloud.rwx.com/mint/org/runs/123",
			"https://cloud.rwx.com/mint/org/runs/456",
			"run-id-789",
		}
		expectedText := "multiple runs processed"

		mockAPI := &mocks.API{
			MockMcpGetRunTestFailures: func(req api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error) {
				require.Equal(t, expectedRunUrls, req.RunUrls)
				return &api.McpTextResult{Type: "text", Text: expectedText}, nil
			},
		}

		setup := setupMCPTest(t, mockAPI)
		defer setup.cleanup()

		result, err := setup.session.CallTool(setup.ctx, &gomcp.CallToolParams{
			Name: "get_run_test_failures",
			Arguments: map[string]any{
				"run_urls": expectedRunUrls,
			},
		})
		require.NoError(t, err)
		expectTextResult(t, result, expectedText)
	})

	t.Run("handles API error", func(t *testing.T) {
		expectedError := errors.New("run not found")

		mockAPI := &mocks.API{
			MockMcpGetRunTestFailures: func(req api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error) {
				return nil, expectedError
			},
		}

		setup := setupMCPTest(t, mockAPI)
		defer setup.cleanup()

		result, err := setup.session.CallTool(setup.ctx, &gomcp.CallToolParams{
			Name: "get_run_test_failures",
			Arguments: map[string]any{
				"run_urls": []string{"https://cloud.rwx.com/mint/org/runs/nonexistent"},
			},
		})
		require.NoError(t, err)
		expectErrorResult(t, result, "run not found")
	})

	t.Run("handles empty run URLs", func(t *testing.T) {
		expectedText := "No runs provided"

		mockAPI := &mocks.API{
			MockMcpGetRunTestFailures: func(req api.McpGetRunTestFailuresRequest) (*api.McpTextResult, error) {
				require.Equal(t, []string{}, req.RunUrls)
				return &api.McpTextResult{Type: "text", Text: expectedText}, nil
			},
		}

		setup := setupMCPTest(t, mockAPI)
		defer setup.cleanup()

		result, err := setup.session.CallTool(setup.ctx, &gomcp.CallToolParams{
			Name: "get_run_test_failures",
			Arguments: map[string]any{
				"run_urls": []string{},
			},
		})
		require.NoError(t, err)
		expectTextResult(t, result, expectedText)
	})
}
