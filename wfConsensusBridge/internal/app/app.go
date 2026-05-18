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
			NacosKV:  map[string]string{},
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
	case model.CommandNacosPut:
		if strings.TrimSpace(tx.Payload.Key) == "" {
			return 1, "nacos key required"
		}
	case model.CommandNacosDelete:
		if strings.TrimSpace(tx.Payload.Key) == "" {
			return 1, "nacos key required"
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
	if tx.Type == model.CommandNacosPut || tx.Type == model.CommandNacosDelete {
		return 0, "ok"
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

	a.ensureNacosKVLocked()

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

	a.ensureNacosKVLocked()

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

func (a *Application) RecordMismatched(tx model.Tx, result model.WorkflowConsensus, preparedHash string, finalizedHash string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ensureNacosKVLocked()

	rec := a.state.Commands[tx.TxID]
	rec.Tx = tx
	rec.ConsensusStatus = "accepted"
	rec.ExecutionStatus = "mismatched"
	rec.Result = result
	rec.ResultBody = consensusDigest(normalizeConsensusResult(tx, result))
	rec.ResultHash = finalizedHash
	rec.PreparedResultHash = preparedHash
	rec.FinalizedResultHash = finalizedHash
	rec.ErrorMessage = fmt.Sprintf("proposer execution result mismatch: prepared=%s finalized=%s", preparedHash, finalizedHash)
	a.state.Commands[tx.TxID] = rec
}

func (a *Application) ExecutionMatches(tx model.Tx, result model.WorkflowConsensus) (bool, string, string) {
	if tx.Execution == nil {
		return true, "", consensusHash(normalizeConsensusResult(tx, result))
	}
	preparedHash := consensusHash(normalizeConsensusResult(tx, tx.Execution.Result))
	finalizedHash := consensusHash(normalizeConsensusResult(tx, result))
	return preparedHash == finalizedHash, preparedHash, finalizedHash
}

/**
Java Nacos 通过 Tendermint broadcast_tx_commit 提交 nacos_put / nacos_delete，共识完成后在 abciapp.Application.FinalizeBlock 里分支调用 ApplyNacosTx。
因此：链上确认的命名数据 = 这里的 NacosKV；abci_query 的 /nacos/key、/nacos/prefix 读的就是这份内存状态（在 Commit 之后 AppHash 也会带上它的摘要）。
*/

// 在区块敲定（FinalizeBlock）时，把一笔已校验过的Nacos写交易落到应用内存状态；更新NacosKV（命名数据的KV存储），并在Commands里记一笔命令流水，供查询与摘要使用
func (a *Application) ApplyNacosTx(tx model.Tx) (uint32, string) {
	// 独占访问整个state，避免与CheckTx、Query、Commit等并发读写打架
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ensureNacosKVLocked()
	key := strings.TrimSpace(tx.Payload.Key)
	rec := a.state.Commands[tx.TxID]
	// 记下原始交易，并把共识侧视为已接受，执行侧标为成功（Nacos交易不走wfEngine，这里相当于链上执行已成功）
	rec.Tx = tx
	rec.ConsensusStatus = "accepted"
	rec.ExecutionStatus = "succeeded"

	switch tx.Type {
	case model.CommandNacosPut:
		// 把 tx.Payload.Value 写入 NacosKV[key]；ResultBody/ResultHash 与 value 对齐，便于对外展示或审计摘要。
		a.state.NacosKV[key] = tx.Payload.Value
		rec.ResultBody = tx.Payload.Value
		rec.ResultHash = hashString(tx.Payload.Value)
	case model.CommandNacosDelete:
		// 从 NacosKV 删除该 key；结果体置空，哈希对空串做 hashString("")。
		delete(a.state.NacosKV, key)
		rec.ResultBody = ""
		rec.ResultHash = hashString("")
	default:
		return 1, "unsupported nacos tx type"
	}
	a.state.Commands[tx.TxID] = rec
	return 0, "ok"
}

func (a *Application) Commit(height int64) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ensureNacosKVLocked()
	a.state.Height = height
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", a.state.Height, a.snapshotDigestLocked())))
	a.state.AppHash = hex.EncodeToString(digest[:])
	return digest[:]
}

// 在已持有a.mu锁的情况下调用，避免和其他goroutine并发改state,把当前应用状态里两块核心数据——所有命令记录 Commands 和 Nacos 键值 NacosKV——序列化成 一段稳定的 JSON 字符串，用作「状态快照」的文本形式，供 Commit 里计算 AppHash（与高度一起参与 sha256）。
func (a *Application) snapshotDigestLocked() string {
	payload, _ := json.Marshal(struct {
		Commands map[string]model.CommandRecord `json:"commands"`
		NacosKV  map[string]string              `json:"nacosKv"`
	}{
		Commands: a.state.Commands,
		NacosKV:  a.state.NacosKV,
	})
	return string(payload)
}

func (a *Application) State() model.AppState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// 读 NacosKV 里指定 key 的值，用于 abci_query 的 /nacos/key 查询
func (a *Application) NacosValue(key string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	value, ok := a.state.NacosKV[strings.TrimSpace(key)]
	return value, ok
}

// 按 key 前缀在 NacosKV 里做一次只读扫描，返回所有「key 以 prefix 开头」的条目（key → value 字符串）。语义上类似 RangeQuery / 前缀枚举。
func (a *Application) NacosValuesByPrefix(prefix string) map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	out := make(map[string]string)
	for key, value := range a.state.NacosKV {
		if strings.HasPrefix(key, prefix) {
			out[key] = value
		}
	}
	return out
}

// 保证 a.state.NacosKV 永远是一个已分配的空 map，而不是 nil
// 调用方应在已持有a.mu锁的情况下调用，避免和其他goroutine并发改state
func (a *Application) ensureNacosKVLocked() {
	if a.state.NacosKV == nil {
		a.state.NacosKV = map[string]string{}
	}
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func consensusHash(result model.WorkflowConsensus) string {
	return hashString(consensusDigest(result))
}

func consensusDigest(result model.WorkflowConsensus) string {
	b, _ := json.Marshal(result)
	return string(b)
}

func normalizeConsensusResult(tx model.Tx, result model.WorkflowConsensus) model.WorkflowConsensus {
	if result.OID == "" {
		result.OID = tx.OID
	}
	if result.DeploymentName == "" {
		result.DeploymentName = tx.Payload.DeploymentName
	}
	if result.ServiceTaskResult == "" {
		result.ServiceTaskResult = tx.Payload.ServiceTaskResult
	}
	return result
}
