package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	abciserver "github.com/tendermint/tendermint/abci/server"
	tmlog "github.com/tendermint/tendermint/libs/log"

	"wfconsensusbridge/internal/abciapp"
	"wfconsensusbridge/internal/app"
	"wfconsensusbridge/internal/executor"
)

func main() {
	// 读取环境变量
	abciAddr := getenv("WF_ABCI_ADDR", "tcp://127.0.0.1:26658") // ABCI socket地址
	httpAddr := getenv("WF_HTTP_ADDR", ":8787") // HTTP监听地址
	wfBase := getenv("WF_ENGINE_BASE", "http://127.0.0.1:8888") // 工作流引擎基础URL，wfEngine地址，确定一下

	// 组装核心对象
	core := app.New() // 创建状态机核心（交易验证、入队、状态管理）
	exec := executor.New(core, wfBase) // 创建执行器（从队列取交易并调用wfEngine）

	// 启后台worker，每2秒从队列取交易并执行，异步消费一条pending交易并执行（deploy/instance/complete）
	// 也就是每隔2s做一次：去队列里看看有没有pending（待执行）交易，有的话就执行；有就拿一条出来执行（调用wfEngine）；没有就什么都不做，等下一轮
	// 交易先经过Tendermint共识后，abciapp.DeliverTx()会把它记到app的状态里，并标记pending
	go func() { // 定时执行器，每200ms从队列取交易并执行
		tk := time.NewTicker(200 * time.Millisecond)
		defer tk.Stop()
		for range tk.C {
			_, _ = exec.RunOnce(context.Background())  //这个函数具体是怎么执行的
		}
	}()

	// 启动ABCI服务，监听ABCI socket地址，接收Tendermint共识交易，并调用abciapp.DeliverTx()处理
	s := abciserver.NewSocketServer(abciAddr, abciapp.New(core))
	s.SetLogger(tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout)))
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
	defer s.Stop()

	// 启HTTP服务，监听HTTP地址，提供状态查询接口
	mux := http.NewServeMux()


	// POST /grafana/wfRequest/instance —— 仅分配 Oid（及建议 txId），不写应用状态、不经 DeliverTx。
	// 实例若要「与 deploy 一样进块、多节点一致」，客户端必须用返回的 oid/txId 组装 JSON 后 broadcast_tx_commit。
	mux.HandleFunc("/grafana/wfRequest/instance", func(w http.ResponseWriter, r *http.Request) {
		var req instanceRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		deploymentName := strings.TrimSpace(req.DeploymentName)
		if deploymentName == "" {
			writeJSON(w, http.StatusBadRequest, instanceResponse{
				Code: 400,
				Body: "deploymentName is required",
			})
			return
		}

		oid := fmt.Sprintf("%s@%s", deploymentName, newID())
		txID := newID()

		writeJSON(w, http.StatusOK, instanceResponse{
			Code: 200,
			Body: "alloc_only: build instance tx with Oid and txId, then broadcast_tx_commit",
			Oid:  oid,
			TxID: txID,
		})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(core.State())
	})
	go func() {
		if err := http.ListenAndServe(httpAddr, mux); err != nil {
			log.Fatal(err)
		}
	}()

	log.Printf("ABCI on %s, HTTP on %s, wfEngine=%s", abciAddr, httpAddr, wfBase)
	select {}
}

func getenv(k string, d string) string {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	return v
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return false
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type instanceRequest struct {
	DeploymentName      string `json:"deploymentName"`
	BusinessData        string `json:"businessData"`
	StaticAllocationTable string `json:"staticAllocationTable"`
}

type instanceResponse struct {
	Code int    `json:"code"`
	Body string `json:"body"`
	Oid  string `json:"Oid"`
	TxID string `json:"txId,omitempty"`
}

func defaultJSON(v string) string {
	if strings.TrimSpace(v) == "" {
		return "{}"
	}
	return v
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
