package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	abciserver "github.com/cometbft/cometbft/abci/server"
	tmlog "github.com/cometbft/cometbft/libs/log"

	"wfconsensusbridge/internal/abciapp"
	"wfconsensusbridge/internal/app"
	"wfconsensusbridge/internal/executor"
	"wfconsensusbridge/internal/gateway"
	"wfconsensusbridge/internal/nacos"
)

func main() {
	// 读取环境变量
	abciAddr := getenv("WF_ABCI_ADDR", "tcp://127.0.0.1:26658") // ABCI socket地址
	httpAddr := getenv("WF_HTTP_ADDR", ":8787")                 // 状态查询HTTP监听地址
	gatewayAddr := getenv("WF_GATEWAY_ADDR", ":8999")           // 服务发现与转发HTTP监听地址
	wfBase := getenv("WF_ENGINE_BASE", "http://127.0.0.1:8888") // 工作流引擎基础URL，wfEngine地址，确定一下
	nacosBase := getenv("WF_NACOS_BASE", "http://127.0.0.1:8848")
	nacosNamespace := getenv("WF_NACOS_NAMESPACE", "public")
	nacosDefaultGroup := getenv("WF_NACOS_GROUP", "DEFAULT_GROUP")

	// 组装核心对象
	core := app.New()            // 创建状态机核心（交易验证、提案校验、状态管理）
	exec := executor.New(wfBase) // 创建执行器（负责 proposer 预执行和最终回放）
	nacosClient := nacos.NewClient(nacosBase, nacosNamespace, &http.Client{Timeout: 10 * time.Second})
	gw := gateway.New(nacosClient, nacosDefaultGroup, &http.Client{Timeout: 20 * time.Second})

	// 启动ABCI服务，监听ABCI socket地址，接收 CometBFT ABCI++ 请求
	s := abciserver.NewSocketServer(abciAddr, abciapp.New(core, exec))
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

	gatewayMux := http.NewServeMux()
	gatewayMux.HandleFunc("/grafana/run", gw.HandleRun)
	gatewayMux.HandleFunc("/grafana/serviceRegister", gw.HandleServiceRegister)
	gatewayMux.HandleFunc("/grafana/serviceDeregister", gw.HandleServiceDeregister)
	gatewayMux.HandleFunc("/grafana/serviceResolve", gw.HandleServiceResolve)
	go func() {
		if err := http.ListenAndServe(gatewayAddr, gatewayMux); err != nil {
			log.Fatal(err)
		}
	}()

	log.Printf("ABCI on %s, state HTTP on %s, gateway HTTP on %s, wfEngine=%s, nacos=%s", abciAddr, httpAddr, gatewayAddr, wfBase, nacosBase)
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
	DeploymentName        string `json:"deploymentName"`
	BusinessData          string `json:"businessData"`
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
