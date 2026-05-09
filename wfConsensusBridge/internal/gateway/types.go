package gateway

import "encoding/json"

type RunRequest struct {
	ConsumerName string `json:"s-consumerName"`
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
