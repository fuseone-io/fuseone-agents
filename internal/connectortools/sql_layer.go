package connectortools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

type sqlRunArgs struct {
	TemplateID string         `json:"template_id"`
	Parameters map[string]any `json:"parameters"`
}

func (l *Layer) invokeSQLNative(
	ctx context.Context, instance Instance, op connectors.Operation, call engine.Call,
) (engine.ToolResult, error) {
	if l.sql == nil || op.ID != "sql.run_query_template" {
		return failed(CodeConnectorUnavailable), nil
	}
	args, ok := decodeSQLRunArgs(call.Args)
	if !ok {
		return failed(CodeConnectorBadArguments), nil
	}
	tpl, found := instance.SQL.Template(args.TemplateID)
	if !found {
		return failed(CodeConnectorBadArguments), nil
	}
	// Validate against the configured template before asking Vault to mint a
	// credential. SQLRuntime repeats this against the authoritative settings
	// read because configuration may change between these two boundaries.
	if _, err := bindParameters(tpl, args.Parameters); err != nil {
		return failed(CodeConnectorBadArguments), nil
	}

	result, err := l.sql.RunBound(
		ctx, instance.Name, args.TemplateID, call.ContractDigest, call.Scope, args.Parameters)
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return engine.ToolResult{}, err
	}
	if err != nil {
		// A failed query records whether authority was issued and returned, not
		// partial database data. This also keeps a shape-too-wide result from
		// being persisted past the template byte bound it failed.
		result.Columns = nil
		result.Rows = nil
	}
	stored, storeErr := l.storeJSON(ctx, call, domain.NewLabels(domain.LabelUntrusted), result)
	if storeErr != nil {
		return engine.ToolResult{}, storeErr
	}
	if err != nil {
		stored.Failed = true
		stored.ErrorCode = CodeConnectorUpstreamFailed
	}
	return stored, nil
}

func decodeSQLRunArgs(raw []byte) (sqlRunArgs, bool) {
	var args sqlRunArgs
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&args); err != nil {
		return sqlRunArgs{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sqlRunArgs{}, false
	}
	if !templateID.MatchString(args.TemplateID) || args.Parameters == nil {
		return sqlRunArgs{}, false
	}
	return args, true
}
