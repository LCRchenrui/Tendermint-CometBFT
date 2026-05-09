package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		},
	}
}

func (a *Application) CheckTx(tx model.Tx) (uint32, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if strings.TrimSpace(tx.TxID) == "" {
		return 1, "txId is required"
	}
	if _, ok := a.state.Commands[tx.TxID]; ok {
		return 1, "duplicate txId"
	}
	return validateTx(tx)
}

func validateTx(tx model.Tx) (uint32, string) {
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

	if tx.Execution != nil {
		if strings.TrimSpace(tx.Execution.Result.OID) == "" {
			tx.Execution.Result.OID = tx.OID
		}
		if tx.Type != model.CommandDeploy && tx.Execution.Result.OID == "" {
			return 1, "prepared tx missing execution oid"
		}
	}
	return 0, "ok"
}

func (a *Application) ValidatePreparedTx(tx model.Tx) (uint32, string) {
	if code, log := validateTx(tx); code != 0 {
		return code, log
	}
	if tx.Execution == nil {
		return 1, "proposal tx missing execution payload"
	}
	if tx.Execution.Result.OID == "" && tx.OID != "" {
		return 1, "execution payload missing oid"
	}
	if tx.Execution.Result.DeploymentName == "" {
		tx.Execution.Result.DeploymentName = tx.Payload.DeploymentName
	}
	return 0, "ok"
}

func (a *Application) RecordPrepared(tx model.Tx) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rec := a.state.Commands[tx.TxID]
	rec.Tx = tx
	rec.ConsensusStatus = "prepared"
	rec.ExecutionStatus = "prepared"
	if tx.Execution != nil {
		rec.Result = tx.Execution.Result
		rec.ResultBody = consensusDigest(tx.Execution.Result)
		rec.ResultHash = hashString(rec.ResultBody)
	}
	a.state.Commands[tx.TxID] = rec
}

func (a *Application) RecordFinalized(tx model.Tx, ok bool, result model.WorkflowConsensus, errMsg string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rec := a.state.Commands[tx.TxID]
	rec.Tx = tx
	rec.ConsensusStatus = "accepted"
	if ok {
		rec.ExecutionStatus = "succeeded"
		rec.Result = result
		rec.ResultBody = consensusDigest(result)
		rec.ResultHash = hashString(rec.ResultBody)
		rec.ErrorMessage = ""
	} else {
		rec.ExecutionStatus = "failed"
		rec.ErrorMessage = errMsg
	}
	a.state.Commands[tx.TxID] = rec
}

func (a *Application) Commit(height int64) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Height = height
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", a.state.Height, a.snapshotDigestLocked())))
	a.state.AppHash = hex.EncodeToString(digest[:])
	return digest[:]
}

func (a *Application) snapshotDigestLocked() string {
	payload, _ := json.Marshal(a.state.Commands)
	return string(payload)
}

func (a *Application) State() model.AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func consensusDigest(result model.WorkflowConsensus) string {
	b, _ := json.Marshal(result)
	return string(b)
}
