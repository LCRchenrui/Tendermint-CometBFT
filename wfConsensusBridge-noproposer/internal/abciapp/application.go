package abciapp

// 实现“ABCI应用层”，把Tendermint ABCI回调映射到app.application的对应方法，也就是把Tendermint的标准回调（BeginBlock/CheckTx/DeliverTx/Commit/Query）转发给你自己的状态机 core（internal/app）

import (
	"encoding/json"
	"fmt"
	"sync"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	"wfconsensusbridge/internal/app"
	"wfconsensusbridge/internal/model"
)

type Application struct {
	abcitypes.BaseApplication
	core *app.Application
	mu   sync.Mutex
	h    int64
}

func New(core *app.Application) *Application {
	return &Application{core: core}
}

func (a *Application) BeginBlock(req abcitypes.RequestBeginBlock) abcitypes.ResponseBeginBlock {
	a.mu.Lock()
	a.h = req.Header.Height
	a.mu.Unlock()
	return abcitypes.ResponseBeginBlock{}
}

func (a *Application) CheckTx(req abcitypes.RequestCheckTx) abcitypes.ResponseCheckTx {
	var tx model.Tx
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return abcitypes.ResponseCheckTx{Code: 1, Log: err.Error()}
	}
	code, log := a.core.CheckTx(tx)
	return abcitypes.ResponseCheckTx{Code: code, Log: log}
}

func (a *Application) DeliverTx(req abcitypes.RequestDeliverTx) abcitypes.ResponseDeliverTx {
	var tx model.Tx
	if err := json.Unmarshal(req.Tx, &tx); err != nil {
		return abcitypes.ResponseDeliverTx{Code: 1, Log: err.Error()}
	}
	code, log := a.core.DeliverTx(tx)
	return abcitypes.ResponseDeliverTx{Code: code, Log: log}
}

func (a *Application) Commit() abcitypes.ResponseCommit {
	a.mu.Lock()
	h := a.h
	a.mu.Unlock()
	return abcitypes.ResponseCommit{Data: a.core.Commit(h)}
}

func (a *Application) Query(req abcitypes.RequestQuery) abcitypes.ResponseQuery {
	if req.Path == "/state" {
		b, _ := json.Marshal(a.core.State())
		return abcitypes.ResponseQuery{Code: 0, Value: b}
	}
	return abcitypes.ResponseQuery{Code: 1, Log: fmt.Sprintf("unsupported path %s", req.Path)}
}
