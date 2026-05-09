package abciapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

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

func (a *Application) CheckTx(_ context.Context, req *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error) {
	var tx model.Tx
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return &abcitypes.ResponseCheckTx{Code: 1, Log: err.Error()}, nil
	}
	code, log := a.core.CheckTx(tx)
	return &abcitypes.ResponseCheckTx{Code: code, Log: log}, nil
}

func (a *Application) PrepareProposal(ctx context.Context, req *abcitypes.RequestPrepareProposal) (*abcitypes.ResponsePrepareProposal, error) {
	var prepared [][]byte
	log.Printf("PrepareProposal: height=%d, txCount=%d", req.Height, len(req.Txs))
	for _, raw := range req.Txs {
		var tx model.Tx
		if err := json.Unmarshal(raw, &tx); err != nil {
			log.Printf("PrepareProposal: unmarshal error: %v", err)
			continue
		}
		log.Printf("PrepareProposal: txId=%s, type=%s", tx.TxID, tx.Type)
		if code, logStr := a.core.CheckTx(tx); code != 0 {
			log.Printf("PrepareProposal: CheckTx failed for %s: %s", tx.TxID, logStr)
			continue
		}
		preparedTx, err := a.exec.PrepareTx(ctx, tx, req.Height)
		if err != nil {
			log.Printf("PrepareProposal: PrepareTx failed for %s: %v", tx.TxID, err)
			continue
		}
		encoded, err := json.Marshal(preparedTx)
		if err != nil {
			log.Printf("PrepareProposal: marshal error for %s: %v", tx.TxID, err)
			continue
		}
		a.core.RecordPrepared(preparedTx)
		prepared = append(prepared, encoded)
		log.Printf("PrepareProposal: tx %s prepared successfully", tx.TxID)
	}
	log.Printf("PrepareProposal: returning %d prepared txs", len(prepared))
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

func (a *Application) Query(_ context.Context, req *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error) {
	if req.Path == "/state" {
		b, _ := json.Marshal(a.core.State())
		return &abcitypes.ResponseQuery{Code: 0, Value: b}, nil
	}
	return &abcitypes.ResponseQuery{Code: 1, Log: fmt.Sprintf("unsupported path %s", req.Path)}, nil
}
