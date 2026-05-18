package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"wfconsensusbridge/internal/model"
)

// 实现的事工作流服务任务分布式锁的二次确认通知：当一笔工作流交易在FinalizeBlock里执行成功或需要回滚时，向业务服务暴露的commit/rollback HTTP接口发POST，告诉对方这笔链上交易最终成功了，请提交锁或失败了/不一致，请回滚锁
// 和Nacos KV无关，只处理wfEngine执行结果里携带的锁信息


const (
	lockActionCommit   = "commit"
	lockActionRollback = "rollback"
)

type serviceTaskExecutionResult struct {
	Status bool   `json:"status"`
	Body   string `json:"body"`
}

type serviceRunResponse struct {
	Provider struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
	} `json:"provider"`
	Data    json.RawMessage `json:"data"`
	RawBody string          `json:"rawBody"`
}

type serviceLockAction struct {
	TaskName    string
	LockToken   string
	CommitURL   string
	RollbackURL string
}

// 它表示交易成功后，通知服务端 commit
func (e *Executor) CommitServiceLocks(ctx context.Context, tx model.Tx, result model.WorkflowConsensus) error {
	return e.notifyServiceLocks(ctx, tx, result, lockActionCommit, "succeeded")
}

// 它表示交易失败或 mismatch 后，通知服务端 rollback
func (e *Executor) RollbackServiceLocks(ctx context.Context, tx model.Tx, result model.WorkflowConsensus, reason string) error {
	status := strings.TrimSpace(reason)
	if status == "" {
		status = "failed"
	}
	return e.notifyServiceLocks(ctx, tx, result, lockActionRollback, status)
}

func (e *Executor) notifyServiceLocks(ctx context.Context, tx model.Tx, result model.WorkflowConsensus, action string, status string) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("WF_SERVICE_LOCK_NOTIFY")), "false") {
		return nil
	}
	actions, err := extractServiceLockActions(tx, result)
	if err != nil {
		return err
	}
	for _, lockAction := range actions {
		target := lockAction.CommitURL
		if action == lockActionRollback {
			target = lockAction.RollbackURL
		}
		if strings.TrimSpace(target) == "" {
			continue
		}
		if err := e.notifyServiceLock(ctx, target, tx, lockAction, action, status); err != nil {
			return err
		}
	}
	return nil
}

// 真正通知服务端
/**
当真实服务收到后
commit:
  根据 txId + lockToken 找 pending 数据
  正式修改数据库
  释放锁

rollback:
  删除 pending 数据
  释放锁
*/
func (e *Executor) notifyServiceLock(ctx context.Context, target string, tx model.Tx, lockAction serviceLockAction, action string, status string) error {
	payload := map[string]any{
		"txId":       tx.TxID,
		"oid":        tx.OID,
		"taskName":   lockAction.TaskName,
		"lockToken":  lockAction.LockToken,
		"action":     action,
		"txStatus":   status,
		"notifiedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.cli.Do(req)
	if err != nil {
		return fmt.Errorf("notify service lock %s %s: %w", action, target, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify service lock %s %s -> %d: %s", action, target, resp.StatusCode, string(raw))
	}
	return nil
}

// 这个文件怎么找到要通知哪个服务：从result.ServiceTaskResult、tx.Execution.Result.ServiceTaskResult和tx.Payload.ServiceTaskResult里面解析服务结果
func extractServiceLockActions(tx model.Tx, result model.WorkflowConsensus) ([]serviceLockAction, error) {
	serviceTaskResult := strings.TrimSpace(result.ServiceTaskResult)
	if serviceTaskResult == "" && tx.Execution != nil {
		serviceTaskResult = strings.TrimSpace(tx.Execution.Result.ServiceTaskResult)
	}
	if serviceTaskResult == "" {
		serviceTaskResult = strings.TrimSpace(tx.Payload.ServiceTaskResult)
	}
	if serviceTaskResult == "" || serviceTaskResult == "{}" || serviceTaskResult == "null" {
		return nil, nil
	}
	var taskResults map[string]serviceTaskExecutionResult
	if err := json.Unmarshal([]byte(serviceTaskResult), &taskResults); err != nil {
		return nil, fmt.Errorf("decode serviceTaskResultJson for lock notification: %w", err)
	}

	out := make([]serviceLockAction, 0)
	seen := map[string]struct{}{}
	for taskName, taskResult := range taskResults {
		if !taskResult.Status || strings.TrimSpace(taskResult.Body) == "" {
			continue
		}
		locks := extractLocksFromGatewayBody(taskName, taskResult.Body)
		for _, lockAction := range locks {
			key := lockAction.TaskName + "|" + lockAction.LockToken + "|" + lockAction.CommitURL + "|" + lockAction.RollbackURL
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, lockAction)
		}
	}
	return out, nil
}

func extractLocksFromGatewayBody(taskName string, body string) []serviceLockAction {
	var run serviceRunResponse
	if err := json.Unmarshal([]byte(body), &run); err != nil {
		return extractLocksFromRawJSON(taskName, "", body)
	}

	baseURL := providerBaseURL(run)
	out := make([]serviceLockAction, 0)
	out = append(out, extractLocksFromRawJSON(taskName, baseURL, string(run.Data))...)
	out = append(out, extractLocksFromRawJSON(taskName, baseURL, run.RawBody)...)
	return out
}

func providerBaseURL(run serviceRunResponse) string {
	if strings.TrimSpace(run.Provider.IP) == "" || run.Provider.Port == 0 {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", strings.TrimSpace(run.Provider.IP), run.Provider.Port)
}

func extractLocksFromRawJSON(taskName string, baseURL string, raw string) []serviceLockAction {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return extractLocksFromValue(taskName, baseURL, value)
}

func extractLocksFromValue(taskName string, baseURL string, value any) []serviceLockAction {
	switch typed := value.(type) {
	case map[string]any:
		out := make([]serviceLockAction, 0)
		if action, ok := lockActionFromMap(taskName, baseURL, typed); ok {
			out = append(out, action)
		}
		for _, key := range []string{"lock", "serviceLock", "txLock"} {
			if nested, ok := typed[key]; ok {
				out = append(out, extractLocksFromValue(taskName, baseURL, nested)...)
			}
		}
		for _, key := range []string{"locks", "serviceLocks", "txLocks"} {
			if nested, ok := typed[key]; ok {
				out = append(out, extractLocksFromValue(taskName, baseURL, nested)...)
			}
		}
		return out
	case []any:
		out := make([]serviceLockAction, 0)
		for _, item := range typed {
			out = append(out, extractLocksFromValue(taskName, baseURL, item)...)
		}
		return out
	default:
		return nil
	}
}

func lockActionFromMap(taskName string, baseURL string, raw map[string]any) (serviceLockAction, bool) {
	token := firstString(raw, "lockToken", "token", "lockId", "lockID")
	commitURL := firstString(raw, "commitUrl", "commitURL", "commitPath")
	rollbackURL := firstString(raw, "rollbackUrl", "rollbackURL", "rollbackPath")

	if token == "" && commitURL == "" && rollbackURL == "" {
		return serviceLockAction{}, false
	}
	if commitURL == "" && baseURL != "" {
		commitURL = "/commit"
	}
	if rollbackURL == "" && baseURL != "" {
		rollbackURL = "/rollback"
	}
	return serviceLockAction{
		TaskName:    taskName,
		LockToken:   token,
		CommitURL:   absoluteServiceURL(baseURL, commitURL),
		RollbackURL: absoluteServiceURL(baseURL, rollbackURL),
	}, true
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			out := strings.TrimSpace(fmt.Sprint(value))
			if out != "" && out != "<nil>" {
				return out
			}
		}
	}
	return ""
}

func absoluteServiceURL(baseURL string, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.IsAbs() {
		return raw
	}
	if strings.TrimSpace(baseURL) == "" {
		return raw
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return strings.TrimRight(baseURL, "/") + raw
}
