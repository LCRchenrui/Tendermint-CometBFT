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

// 在交易被写进正式提案之前，先调一次wfEngine做预执行，把执行结果封进交易本身，这样其他节点在ProcessProposal里能校验、在FinalizeBlock里能按同一套数据再执行/回放
func (e *Executor) PrepareTx(ctx context.Context, tx model.Tx, height int64) (model.Tx, error) {
	// 按交易类型发 HTTP 到 wfEngine（deploy / instance / complete），第三个参数 true 表示要 解析 wfEngine 返回的共识载荷（WorkflowConsensus 等），失败则原样返回 tx 和 err（注意：这里仍返回原 tx，但带 error，上层 PrepareProposal 里会 continue 丢掉这笔）。
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
	result, err := e.ReplayTx(ctx, tx)
	if err != nil {
		return model.WorkflowConsensus{}, err
	}
	if e.autoFlush {
		if err := e.FlushTx(ctx, tx); err != nil {
			return model.WorkflowConsensus{}, err
		}
	}
	return result, nil
}

func (e *Executor) ReplayTx(ctx context.Context, tx model.Tx) (model.WorkflowConsensus, error) {
	if tx.Execution != nil && tx.Payload.ServiceTaskResult == "" {
		tx.Payload.ServiceTaskResult = tx.Execution.Result.ServiceTaskResult
	}
	result, err := e.execute(ctx, tx, true)
	if err != nil {
		return model.WorkflowConsensus{}, err
	}
	return result, nil
}

func (e *Executor) FlushTx(ctx context.Context, tx model.Tx) error {
	if !e.autoFlush {
		return nil
	}
	return e.flush(ctx, tx)
}

// 根据交易类型，把统一的共识交易tx翻译成对应的wfEngine HTTP请求并发送
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
			"txId":                  tx.TxID,   			// wfConsensusBridge 调 wfEngine 时，把 txId 一起传过去。因为后面的服务要知道这次加锁属于哪一笔交易，后面 commit/rollback 的是哪一笔交易
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
			"txId":             tx.TxID,   			// wfConsensusBridge 调 wfEngine 时，把 txId 一起传过去。因为后面的服务要知道这次加锁属于哪一笔交易，后面 commit/rollback 的是哪一笔交易
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
