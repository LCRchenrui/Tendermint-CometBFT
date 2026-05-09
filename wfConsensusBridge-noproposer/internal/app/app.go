package app

// 实现“应用状态机核心逻辑”

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"wfconsensusbridge/internal/model"
)

type Application struct {
	mu    sync.RWMutex
	state model.AppState
}

func New() *Application {
	return &Application{
		state: model.AppState{
			Commands: map[string]model.CommandRecord{},
			Queue:    []string{},
		},
	}
}

// 检查交易是否合法，主要是检查交易类型、交易ID、交易内容等是否符合要求
func (a *Application) CheckTx(tx model.Tx) (uint32, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if strings.TrimSpace(tx.TxID) == "" {
		return 1, "txId is required"
	}
	if _, ok := a.state.Commands[tx.TxID]; ok {
		return 1, "duplicate txId"
	}
	switch tx.Type {
	case model.CommandDeploy:
		if !strings.HasSuffix(tx.Payload.DeploymentName, ".bpmn") {
			return 1, "deploymentName must end with .bpmn"
		}
	case model.CommandInstance:
		if tx.OID == "" || tx.Payload.DeploymentName == "" {
			return 1, "oid/deploymentName required"
		}
	case model.CommandComplete:
		if tx.OID == "" || tx.Payload.TaskName == "" || tx.Payload.User == "" {
			return 1, "oid/taskName/user required"
		}
	default:
		return 1, "unsupported tx type"
	}
	return 0, "ok"
}

// 处理交易，把交易记到状态机里，并标记为pending
func (a *Application) DeliverTx(tx model.Tx) (uint32, string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.state.Commands[tx.TxID]; ok {
		return 1, "duplicate txId"
	}
	a.state.Commands[tx.TxID] = model.CommandRecord{
		Tx:              tx,
		ConsensusStatus: "accepted",
		ExecutionStatus: "pending",
	}
	a.state.Queue = append(a.state.Queue, tx.TxID)
	return 0, "accepted"
}

// 提交区块时，更新状态机高度，并计算状态机哈希
func (a *Application) Commit(height int64) []byte {
	a.mu.Lock()
	a.state.Height = height
	a.mu.Unlock()

	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", a.state.Height, len(a.state.Commands))))
	return sum[:]
}

// 从队列里取一条pending交易，并标记为in_progress
func (a *Application) ClaimNextPending() (model.CommandRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, txID := range a.state.Queue {
		r := a.state.Commands[txID]
		if r.ExecutionStatus == "pending" {
			r.ExecutionStatus = "in_progress"
			a.state.Commands[txID] = r
			return r, true
		}
	}
	return model.CommandRecord{}, false
}

// 标记交易执行结果，更新状态机里的执行状态和结果
func (a *Application) MarkExecuted(txID string, ok bool, body string, errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	r := a.state.Commands[txID]
	if ok {
		r.ExecutionStatus = "succeeded"
		r.ResultBody = body
		h := sha256.Sum256([]byte(body))
		r.ResultHash = hex.EncodeToString(h[:])
		r.ErrorMessage = ""
	} else {
		r.ExecutionStatus = "failed"
		r.ErrorMessage = errMsg
	}
	a.state.Commands[txID] = r
}

// 获取当前状态机状态
func (a *Application) State() model.AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}
