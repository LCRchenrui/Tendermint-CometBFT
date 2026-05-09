package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"wfconsensusbridge/internal/model"
)

type Executor struct {
	base      string
	cli       *http.Client
	autoFlush bool
}

func New(wfEngineBase string) *Executor {
	autoFlush := strings.ToLower(strings.TrimSpace(os.Getenv("WF_AUTO_FLUSH"))) != "false"
	return &Executor{
		base:      strings.TrimRight(wfEngineBase, "/"),
		cli:       &http.Client{Timeout: 20 * time.Second},
		autoFlush: autoFlush,
	}
}

func (e *Executor) PrepareTx(ctx context.Context, tx model.Tx, height int64) (model.Tx, error) {
	result, err := e.execute(ctx, tx, true)
	if err != nil {
		return tx, err
	}
	tx.Payload.ServiceTaskResult = result.ServiceTaskResult
	tx.Execution = &model.ExecutionEnvelope{
		PreparedAtHeight: height,
		PreparedBy:       "proposer",
		Result:           result,
	}
	return tx, nil
}

func (e *Executor) FinalizeTx(ctx context.Context, tx model.Tx) (model.WorkflowConsensus, error) {
	if tx.Execution != nil && tx.Payload.ServiceTaskResult == "" {
		tx.Payload.ServiceTaskResult = tx.Execution.Result.ServiceTaskResult
	}
	result, err := e.execute(ctx, tx, true)
	if err != nil {
		return model.WorkflowConsensus{}, err
	}
	if e.autoFlush {
		if err := e.flush(ctx, tx); err != nil {
			return model.WorkflowConsensus{}, err
		}
	}
	return result, nil
}

func (e *Executor) execute(ctx context.Context, tx model.Tx, consensusPayload bool) (model.WorkflowConsensus, error) {
	var (
		raw string
		err error
	)
	switch tx.Type {
	case model.CommandDeploy:
		raw, err = e.post(ctx, e.base+"/wfEngine/wfDeploy", map[string]any{
			"deploymentName":   tx.Payload.DeploymentName,
			"fileContent":      tx.Payload.FileContent,
			"signatures":       defaultJSON(tx.Payload.Signatures),
			"consensusPayload": consensusPayload,
		})
	case model.CommandInstance:
		payload := map[string]any{
			"deploymentName":        tx.Payload.DeploymentName,
			"businessData":          tx.Payload.BusinessData,
			"staticAllocationTable": defaultJSON(tx.Payload.StaticAllocation),
			"consensusPayload":      consensusPayload,
		}
		if strings.TrimSpace(tx.Payload.ServiceTaskResult) != "" {
			payload["serviceTaskResultJson"] = tx.Payload.ServiceTaskResult
		}
		raw, err = e.post(ctx, e.base+"/wfEngine/wfInstance/"+tx.OID, payload)
	case model.CommandComplete:
		payload := map[string]any{
			"taskName":         tx.Payload.TaskName,
			"processData":      defaultJSON(tx.Payload.ProcessData),
			"businessData":     tx.Payload.BusinessData,
			"user":             tx.Payload.User,
			"consensusPayload": consensusPayload,
		}
		if strings.TrimSpace(tx.Payload.ServiceTaskResult) != "" {
			payload["serviceTaskResultJson"] = tx.Payload.ServiceTaskResult
		}
		raw, err = e.post(ctx, e.base+"/wfEngine/wfComplete/"+tx.OID, payload)
	default:
		return model.WorkflowConsensus{}, fmt.Errorf("unsupported tx type")
	}
	if err != nil {
		return model.WorkflowConsensus{}, err
	}
	if !consensusPayload {
		return model.WorkflowConsensus{}, nil
	}
	var result model.WorkflowConsensus
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return model.WorkflowConsensus{}, fmt.Errorf("decode wfEngine consensus payload: %w", err)
	}
	if result.OID == "" {
		result.OID = tx.OID
	}
	if result.DeploymentName == "" {
		result.DeploymentName = tx.Payload.DeploymentName
	}
	if result.ServiceTaskResult == "" {
		result.ServiceTaskResult = tx.Payload.ServiceTaskResult
	}
	return result, nil
}

func (e *Executor) post(ctx context.Context, url string, payload map[string]any) (string, error) {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.cli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("wfEngine %d: %s", resp.StatusCode, string(raw))
	}
	return string(raw), nil
}

func (e *Executor) flush(ctx context.Context, tx model.Tx) error {
	oid := flushOID(tx)
	if oid == "" {
		return nil
	}
	_, err := e.post(ctx, e.base+"/wfEngine/flush", map[string]any{
		"oidsString": oid,
	})
	if err != nil {
		return fmt.Errorf("auto flush failed for oid=%s: %w", oid, err)
	}
	return nil
}

func flushOID(tx model.Tx) string {
	switch tx.Type {
	case model.CommandDeploy:
		if strings.ToLower(strings.TrimSpace(os.Getenv("WF_AUTO_FLUSH_DEPLOY"))) == "true" {
			return strings.TrimSpace(tx.Payload.DeploymentName)
		}
		return ""
	case model.CommandInstance, model.CommandComplete:
		return strings.TrimSpace(tx.OID)
	default:
		return ""
	}
}

func defaultJSON(v string) string {
	if strings.TrimSpace(v) == "" {
		return "{}"
	}
	return v
}
