package executor

// 消费pending交易并调用wfEngine HTTP接口
// 按交易类型路由
// 回写执行结果到状态机

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

	"wfconsensusbridge/internal/app"
	"wfconsensusbridge/internal/model"
)

type Executor struct {
	core      *app.Application
	base      string
	cli       *http.Client
	autoFlush bool
}

func New(core *app.Application, wfEngineBase string) *Executor {
	// Default on: avoid requiring manual curl flush during normal operation.
	// Note: flushing deploymentName can collide with Redis SET keys used for deploy signatures,
	// so auto-flush targets instance/complete OIDs by default. Deploy flush is opt-in.
	autoFlush := strings.ToLower(strings.TrimSpace(os.Getenv("WF_AUTO_FLUSH"))) != "false"
	return &Executor{
		core:      core,
		base:      strings.TrimRight(wfEngineBase, "/"),
		cli:       &http.Client{Timeout: 20 * time.Second},
		autoFlush: autoFlush,
	}
}

func (e *Executor) RunOnce(ctx context.Context) (bool, error) {
	rec, ok := e.core.ClaimNextPending()
	if !ok {
		return false, nil
	}
	body, err := e.execute(ctx, rec.Tx)
	if err != nil {
		e.core.MarkExecuted(rec.Tx.TxID, false, "", err.Error())
		return true, err
	}
	if e.autoFlush {
		if err := e.flush(ctx, rec.Tx); err != nil {
			e.core.MarkExecuted(rec.Tx.TxID, false, "", err.Error())
			return true, err
		}
	}
	e.core.MarkExecuted(rec.Tx.TxID, true, body, "")
	return true, nil
}

// 根据交易类型，把统一的共识交易tx翻译成对应的wfEngine HTTP请求并发送
func (e *Executor) execute(ctx context.Context, tx model.Tx) (string, error) {
	switch tx.Type {
	case model.CommandDeploy:
		return e.post(ctx, e.base+"/wfEngine/wfDeploy", map[string]any{
			"deploymentName": tx.Payload.DeploymentName,
			"fileContent":    tx.Payload.FileContent,
			"signatures":     defaultJSON(tx.Payload.Signatures),
		})
	case model.CommandInstance:
		return e.post(ctx, e.base+"/wfEngine/wfInstance/"+tx.OID, map[string]any{
			"deploymentName":        tx.Payload.DeploymentName,
			"businessData":          tx.Payload.BusinessData,
			"staticAllocationTable": defaultJSON(tx.Payload.StaticAllocation),
		})
	case model.CommandComplete:
		return e.post(ctx, e.base+"/wfEngine/wfComplete/"+tx.OID, map[string]any{
			"taskName":     tx.Payload.TaskName,
			"processData":  defaultJSON(tx.Payload.ProcessData),
			"businessData": tx.Payload.BusinessData,
			"user":         tx.Payload.User,
		})
	default:
		return "", fmt.Errorf("unsupported tx type")
	}
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
