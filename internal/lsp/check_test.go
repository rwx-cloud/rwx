package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunCheckProtocolAccessToken(t *testing.T) {
	for _, token := range []string{"resolved-token", ""} {
		t.Run(token, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			serverReader, clientWriter := io.Pipe()
			defer serverReader.Close()
			defer clientWriter.Close()
			conn := newJSONRPCConn(reader, clientWriter)
			server := newJSONRPCConn(serverReader, writer)
			messages := make(chan jsonrpcMessage, 1)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			go conn.readLoop(ctx)
			go func() {
				message, err := server.readMessage()
				if err != nil {
					return
				}
				messages <- message
				response := `{"jsonrpc":"2.0","id":1,"result":{}}`
				_, _ = fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(response), response)
				_, _ = server.readMessage()
			}()

			_, _, err := runCheckProtocol(ctx, conn, t.TempDir(), nil, false, token)
			require.NoError(t, err)
			message := <-messages
			require.Equal(t, "initialize", message.Method)
			var params struct {
				InitializationOptions map[string]string `json:"initializationOptions"`
			}
			require.NoError(t, json.Unmarshal(message.Params, &params))
			require.Equal(t, map[string]string{"accessToken": token}, params.InitializationOptions)
		})
	}
}

func TestOutputCheckMultiLine_NoDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: nil,
		FileCount:   3,
	}

	err := outputCheckMultiLine(&buf, result)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Checked 3 files and found 0 problems.")
}

func TestOutputCheckMultiLine_SingleDiagnostic(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: []CheckDiagnostic{
			{Severity: "error", Message: "unknown key", FilePath: ".rwx/mint.yml", Line: 5, Column: 3},
		},
		FileCount: 1,
	}

	err := outputCheckMultiLine(&buf, result)
	require.NoError(t, err)
	require.Contains(t, buf.String(), ".rwx/mint.yml:5:3  [error]")
	require.Contains(t, buf.String(), "unknown key")
	require.Contains(t, buf.String(), "Checked 1 file and found 1 problem.")
}

func TestOutputCheckMultiLine_MultipleDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: []CheckDiagnostic{
			{Severity: "error", Message: "unknown key", FilePath: ".rwx/a.yml", Line: 1, Column: 1},
			{Severity: "warning", Message: "deprecated", FilePath: ".rwx/b.yml", Line: 10, Column: 5},
		},
		FileCount: 2,
	}

	err := outputCheckMultiLine(&buf, result)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "Checked 2 files and found 2 problems.")
}

func TestOutputCheckOneLine(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: []CheckDiagnostic{
			{Severity: "error", Message: "bad value", FilePath: ".rwx/mint.yml", Line: 3, Column: 7},
		},
		FileCount: 1,
	}

	err := outputCheckOneLine(&buf, result)
	require.NoError(t, err)
	require.Contains(t, buf.String(), "error")
	require.Contains(t, buf.String(), ".rwx/mint.yml:3:7 - bad value")
}

func TestOutputCheckOneLine_NoDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: nil,
		FileCount:   2,
	}

	err := outputCheckOneLine(&buf, result)
	require.NoError(t, err)
	require.Empty(t, buf.String())
}

func TestOutputCheckJSON(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: []CheckDiagnostic{
			{Severity: "error", Message: "test error", FilePath: ".rwx/test.yml", Line: 1, Column: 1},
		},
		FileCount: 1,
	}

	err := outputCheckJSON(&buf, result)
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"Diagnostics"`)
	require.Contains(t, buf.String(), `"FileCount":1`)
	require.Contains(t, buf.String(), `"Severity":"error"`)
}

func TestOutputCheckJSON_EmptyDiagnostics(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: nil,
		FileCount:   0,
	}

	err := outputCheckJSON(&buf, result)
	require.NoError(t, err)
	// Should output empty array, not null
	require.Contains(t, buf.String(), `"Diagnostics":[]`)
	require.Contains(t, buf.String(), `"FileCount":0`)
}

func TestOutputCheckNone(t *testing.T) {
	var buf bytes.Buffer
	result := &CheckResult{
		Diagnostics: []CheckDiagnostic{
			{Severity: "error", Message: "test error", FilePath: ".rwx/test.yml", Line: 1, Column: 1},
		},
		FileCount: 1,
	}

	err := outputCheckResult(&buf, CheckOutputNone, result)
	require.NoError(t, err)
	require.Empty(t, buf.String())
}

func TestNewCheckConfig_ValidFormats(t *testing.T) {
	tests := []struct {
		format   string
		expected CheckOutputFormat
	}{
		{"multiline", CheckOutputMultiLine},
		{"text", CheckOutputMultiLine},
		{"oneline", CheckOutputOneLine},
		{"json", CheckOutputJSON},
		{"none", CheckOutputNone},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			cfg, err := NewCheckConfig("", tt.format, 0, nil, false)
			require.NoError(t, err)
			require.Equal(t, tt.expected, cfg.OutputFormat)
		})
	}
}

func TestNewCheckConfig_InvalidFormat(t *testing.T) {
	_, err := NewCheckConfig("", "invalid", 0, nil, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown output format")
}

func TestParseDiagnosticResult_EmptyItems(t *testing.T) {
	result := []byte(`{"kind":"full","items":[]}`)
	diags, err := parseDiagnosticResult(result, "test.yml")
	require.NoError(t, err)
	require.Empty(t, diags)
}

func TestParseDiagnosticResult_InvalidJSON(t *testing.T) {
	result := []byte(`not json`)
	_, err := parseDiagnosticResult(result, "test.yml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to parse diagnostic response")
}

func TestParseDiagnosticResult_WithItems(t *testing.T) {
	result := []byte(`{"kind":"full","items":[{"severity":1,"message":"unknown key","range":{"start":{"line":4,"character":2},"end":{"line":4,"character":10}}},{"severity":2,"message":"deprecated field","range":{"start":{"line":10,"character":0},"end":{"line":10,"character":5}}}]}`)
	diags, err := parseDiagnosticResult(result, ".rwx/test.yml")
	require.NoError(t, err)

	require.Len(t, diags, 2)
	require.Equal(t, "error", diags[0].Severity)
	require.Equal(t, "unknown key", diags[0].Message)
	require.Equal(t, 5, diags[0].Line)   // 0-based -> 1-based
	require.Equal(t, 3, diags[0].Column) // 0-based -> 1-based
	require.Equal(t, "warning", diags[1].Severity)
}

func TestParseDiagnosticResult_AllSeverities(t *testing.T) {
	result := []byte(`{"kind":"full","items":[
		{"severity":1,"message":"err","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":1}}},
		{"severity":2,"message":"warn","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":1}}},
		{"severity":3,"message":"info","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":1}}},
		{"severity":4,"message":"hint","range":{"start":{"line":3,"character":0},"end":{"line":3,"character":1}}}
	]}`)
	diags, err := parseDiagnosticResult(result, "test.yml")
	require.NoError(t, err)
	require.Len(t, diags, 4)
	require.Equal(t, "error", diags[0].Severity)
	require.Equal(t, "warning", diags[1].Severity)
	require.Equal(t, "info", diags[2].Severity)
	require.Equal(t, "hint", diags[3].Severity)
}

func TestApplyTextEdits_NoEdits(t *testing.T) {
	content := "line one\nline two\nline three"
	result := applyTextEdits(content, nil)
	require.Equal(t, content, result)
}

func TestApplyTextEdits_SingleEdit(t *testing.T) {
	content := "hello world\nfoo bar"
	edits := []textEdit{
		{StartLine: 0, StartChar: 6, EndLine: 0, EndChar: 11, NewText: "earth"},
	}
	result := applyTextEdits(content, edits)
	require.Equal(t, "hello earth\nfoo bar", result)
}

func TestApplyTextEdits_MultipleEditsOnSameLine(t *testing.T) {
	content := "aaa bbb ccc"
	edits := []textEdit{
		{StartLine: 0, StartChar: 0, EndLine: 0, EndChar: 3, NewText: "AAA"},
		{StartLine: 0, StartChar: 8, EndLine: 0, EndChar: 11, NewText: "CCC"},
	}
	result := applyTextEdits(content, edits)
	require.Equal(t, "AAA bbb CCC", result)
}

func TestApplyTextEdits_OverlappingEditsDeduped(t *testing.T) {
	content := "aaa bbb ccc"
	edits := []textEdit{
		{StartLine: 0, StartChar: 4, EndLine: 0, EndChar: 7, NewText: "BBB"},
		{StartLine: 0, StartChar: 4, EndLine: 0, EndChar: 7, NewText: "XXX"},
	}
	result := applyTextEdits(content, edits)
	require.Equal(t, "aaa BBB ccc", result)
}

func TestApplyTextEdits_MultiLineEdit(t *testing.T) {
	content := "line one\nline two\nline three"
	edits := []textEdit{
		{StartLine: 0, StartChar: 5, EndLine: 1, EndChar: 8, NewText: "ONE\nLINE TWO"},
	}
	result := applyTextEdits(content, edits)
	require.Equal(t, "line ONE\nLINE TWO\nline three", result)
}
