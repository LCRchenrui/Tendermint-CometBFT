package abciapp

// 这个application.go是wfConsensusBridge对接CometBFT（ABCI++）的适配层：把链节点发来的ABCI请求，转成对core（内存状态机）和exec（调wfEngine）的调用
import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	"wfconsensusbridge/internal/app"
	"wfconsensusbridge/internal/executor"
	"wfconsensusbridge/internal/model"
)

type Application struct {
	abcitypes.BaseApplication
	core *app.Application
	exec *executor.Executor
}

func New(core *app.Application, exec *executor.Executor) *Application {
	return &Application{core: core, exec: exec}
}

func (a *Application) Info(_ context.Context, _ *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error) {
	state := a.core.State()
	return &abcitypes.ResponseInfo{
		LastBlockHeight:  state.Height,
		LastBlockAppHash: []byte(state.AppHash),
	}, nil
}

// 交易进mempool前，CometBFT会先问：这笔交易能不能收
func (a *Application) CheckTx(_ context.Context, req *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error) {
	var tx model.Tx
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return &abcitypes.ResponseCheckTx{Code: 1, Log: err.Error()}, nil
	}
	code, log := a.core.CheckTx(tx)
	return &abcitypes.ResponseCheckTx{Code: code, Log: log}, nil
}

// 本轮提议者组装候选区块时调用：可以把mempool里的原始交易加工后再放进提案
func (a *Application) PrepareProposal(ctx context.Context, req *abcitypes.RequestPrepareProposal) (*abcitypes.ResponsePrepareProposal, error) {
	var prepared [][]byte
	for _, raw := range req.Txs {
		var tx model.Tx
		if err := json.Unmarshal(raw, &tx); err != nil {
			continue
		}
		// 再跑一遍core.CheckTx，确保交易合法
		if code, _ := a.core.CheckTx(tx); code != 0 {
			continue
		}
		if isNacosTx(tx) {
			// 在应用核心的状态机里登记这笔交易已经按规则进入提案流程，后面ProcessProposal / FinalizeBlock 里会用 ValidatePreparedTx 等对同一笔交易做一致性校验（和下面走 wfEngine 的路径对齐）。
			a.core.RecordPrepared(tx)
			// 把交易编码成字节数组，准备放进提案，放进prepared切片里，后面会发给其他节点。
			prepared = append(prepared, raw)
			continue
		}
		// 预执行wfEngine，把结果写进交易的Execution/ServiceTaskResult字段
		preparedTx, err := a.exec.PrepareTx(ctx, tx, req.Height)
		if err != nil {
			continue
		}
		encoded, err := json.Marshal(preparedTx)
		if err != nil {
			continue
		}
		a.core.RecordPrepared(preparedTx)
		prepared = append(prepared, encoded)
	}
	return &abcitypes.ResponsePrepareProposal{Txs: prepared}, nil
}

func (a *Application) ProcessProposal(_ context.Context, req *abcitypes.RequestProcessProposal) (*abcitypes.ResponseProcessProposal, error) {
	for _, raw := range req.Txs {
		var tx model.Tx
		if err := json.Unmarshal(raw, &tx); err != nil {
			return &abcitypes.ResponseProcessProposal{Status: abcitypes.ResponseProcessProposal_REJECT}, nil
		}
		if code, _ := a.core.ValidatePreparedTx(tx); code != 0 {
			return &abcitypes.ResponseProcessProposal{Status: abcitypes.ResponseProcessProposal_REJECT}, nil
		}
	}
	return &abcitypes.ResponseProcessProposal{Status: abcitypes.ResponseProcessProposal_ACCEPT}, nil
}

func (a *Application) FinalizeBlock(ctx context.Context, req *abcitypes.RequestFinalizeBlock) (*abcitypes.ResponseFinalizeBlock, error) {
	txResults := make([]*abcitypes.ExecTxResult, 0, len(req.Txs))
	for _, raw := range req.Txs {
		var tx model.Tx
		if err := json.Unmarshal(raw, &tx); err != nil {
			txResults = append(txResults, &abcitypes.ExecTxResult{Code: 1, Log: err.Error()})
			continue
		}
		if code, log := a.core.ValidatePreparedTx(tx); code != 0 {
			txResults = append(txResults, &abcitypes.ExecTxResult{Code: code, Log: log})
			continue
		}
		// 是区块已经达成共识，要顺序执行块内每笔交易并给出执行结果时运行：比如nacos的写入/删除，直接调用core.ApplyNacosTx
		if isNacosTx(tx) {
			// 在 app.Application 里真正把这笔交易落到应用状态：在持锁下更新内存里的 NacosKV（put 写入 key/value，delete 删掉 key），并更新 state.Commands[tx.TxID] 里的共识/执行记录（标记为已接受、执行成功等）。返回值 code, log 给 ABCI：0 表示成功，非 0 表示失败
			code, log := a.core.ApplyNacosTx(tx)
			txResults = append(txResults, &abcitypes.ExecTxResult{Code: code, Log: log})
			continue
		}
		result, err := a.exec.FinalizeTx(ctx, tx)
		if err != nil {
			a.core.RecordFinalized(tx, false, model.WorkflowConsensus{}, err.Error())
			txResults = append(txResults, &abcitypes.ExecTxResult{Code: 1, Log: err.Error()})
			continue
		}
		a.core.RecordFinalized(tx, true, result, "")
		txResults = append(txResults, &abcitypes.ExecTxResult{Code: 0, Log: "ok"})
	}
	return &abcitypes.ResponseFinalizeBlock{
		TxResults: txResults,
		AppHash:   a.core.Commit(req.Height),
	}, nil
}

func (a *Application) Commit(_ context.Context, _ *abcitypes.RequestCommit) (*abcitypes.ResponseCommit, error) {
	return &abcitypes.ResponseCommit{}, nil
}


// /state .dump 全状态；/nacos/key 单 key 读；/nacos/prefix 前缀扫并返回有序 JSON 列表。三者都是 只读内存状态，不参与共识写入；写入发生在 FinalizeBlock → ApplyNacosTx 更新 NacosKV 之后，查询才能读到新数据。
func (a *Application) Query(_ context.Context, req *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error) {
	// Query 里，响应 CometBFT 的 abci_query（Java 里 TendermintCrudByHttp.query(...) 调的就是它）
	if req.Path == "/state" {
		b, _ := json.Marshal(a.core.State())
		return &abcitypes.ResponseQuery{Code: 0, Value: b}, nil
	}
	if req.Path == "/nacos/key" {
		key := queryDataString(req.Data)
		if value, ok := a.core.NacosValue(key); ok {
			return &abcitypes.ResponseQuery{Code: 0, Value: []byte(value)}, nil
		}
		return &abcitypes.ResponseQuery{Code: 1, Log: fmt.Sprintf("nacos key not found: %s", key)}, nil
	}
	if req.Path == "/nacos/prefix" {
		prefix := queryDataString(req.Data)
		values := a.core.NacosValuesByPrefix(prefix)
		type record struct {
			Key    string `json:"Key"`
			Record string `json:"Record"`
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		records := make([]record, 0, len(keys))
		for _, key := range keys {
			records = append(records, record{Key: key, Record: values[key]})
		}
		b, _ := json.Marshal(records)
		return &abcitypes.ResponseQuery{Code: 0, Value: b}, nil
	}
	return &abcitypes.ResponseQuery{Code: 1, Log: fmt.Sprintf("unsupported path %s", req.Path)}, nil
}

func isNacosTx(tx model.Tx) bool {
	return tx.Type == model.CommandNacosPut || tx.Type == model.CommandNacosDelete
}

func queryDataString(data []byte) string {
	value := strings.TrimSpace(string(data))
	value = strings.Trim(value, "\"")
	return value
}
