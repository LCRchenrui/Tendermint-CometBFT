package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wfconsensusbridge/internal/nacos"
)

type Gateway struct {
	nacos        *nacos.Client
	defaultGroup string
	cli          *http.Client
	rand         *rand.Rand
}

func New(nc *nacos.Client, defaultGroup string, cli *http.Client) *Gateway {
	return &Gateway{
		nacos:        nc,
		defaultGroup: strings.TrimSpace(defaultGroup),
		cli:          cli,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (g *Gateway) forward(ctx context.Context, req RunRequest) (*RunResponse, error) {
	group := strings.TrimSpace(req.Group)
	if group == "" {
		group = g.defaultGroup
	}

	inst, err := g.resolve(ctx, req.ServiceName, group)
	if err != nil {
		return nil, err
	}

	target, err := buildTarget(inst, req.URL)
	if err != nil {
		return nil, err
	}

	resp, err := g.doForward(ctx, target, req)
	if err != nil {
		return nil, err
	}

	return &RunResponse{
		Code:    resp.Code,
		Message: resp.Message,
		Provider: ProviderInfo{
			ServiceName: req.ServiceName,
			Group:       group,
			IP:          inst.IP,
			Port:        inst.Port,
		},
		Data:    resp.Data,
		RawBody: resp.RawBody,
	}, nil
}

func (g *Gateway) resolve(ctx context.Context, serviceName, group string) (nacos.Instance, error) {
	insts, err := g.nacos.ListInstances(ctx, serviceName, group, true)
	if err != nil {
		return nacos.Instance{}, err
	}
	healthy := make([]nacos.Instance, 0, len(insts))
	for _, inst := range insts {
		if inst.Healthy && inst.Enabled {
			healthy = append(healthy, inst)
		}
	}
	if len(healthy) == 0 {
		return nacos.Instance{}, fmt.Errorf("no healthy instance for %s@%s", serviceName, group)
	}
	return healthy[g.rand.Intn(len(healthy))], nil
}

func buildTarget(inst nacos.Instance, path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("empty service url")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return fmt.Sprintf("http://%s:%d%s", inst.IP, inst.Port, p), nil
}

type forwardResult struct {
	Code    int
	Message string
	Data    json.RawMessage
	RawBody string
}

func (g *Gateway) doForward(ctx context.Context, target string, req RunRequest) (*forwardResult, error) {
	body := strings.TrimSpace(req.Body)
	if body == "" {
		body = "{}"
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(strings.TrimSpace(req.Method)), target, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}

	headers, err := parseHeaders(req.Headers)
	if err != nil {
		return nil, fmt.Errorf("invalid headers: %w", err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	if httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.cli.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	result := &forwardResult{
		Code:    resp.StatusCode,
		Message: "ok",
		RawBody: string(raw),
	}
	if resp.StatusCode >= 300 {
		result.Message = fmt.Sprintf("upstream status %d", resp.StatusCode)
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && json.Valid(trimmed) {
		result.Data = append(json.RawMessage(nil), trimmed...)
	}
	return result, nil
}

func parseHeaders(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err == nil {
		return headers, nil
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	headers = make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers, nil
}
