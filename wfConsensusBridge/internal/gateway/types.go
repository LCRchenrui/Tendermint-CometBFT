package gateway

import "encoding/json"

type RunRequest struct {
	ConsumerName string `json:"s-consumerName"`
	// 新增字段：txId，表示这次服务调用属于哪一笔交易，这是 Go gateway 对 wfEngine 请求体的解析结构。
	// 如果wfEngine发来"s-txId": "c-1001"，会被解析到req.TxID
	TxID         string `json:"s-txId"`
	ServiceName  string `json:"s-serviceName"`
	Group        string `json:"s-group"`
	URL          string `json:"s-url"`
	Method       string `json:"s-method"`
	Headers      string `json:"headers"`
	Body         string `json:"body"`
}

type RunResponse struct {
	Code     int             `json:"code"`
	Message  string          `json:"message"`
	Provider ProviderInfo    `json:"provider,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	RawBody  string          `json:"rawBody,omitempty"`
}

type ProviderInfo struct {
	ServiceName string `json:"serviceName"`
	Group       string `json:"group"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
}
