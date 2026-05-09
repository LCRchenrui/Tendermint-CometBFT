package nacos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	baseURL   string
	namespace string
	cli       *http.Client
}

type Instance struct {
	IP          string            `json:"ip"`
	Port        int               `json:"port"`
	ServiceName string            `json:"serviceName"`
	GroupName   string            `json:"groupName"`
	Healthy     bool              `json:"healthy"`
	Enabled     bool              `json:"enabled"`
	Metadata    map[string]string `json:"metadata"`
}

type RegisterInstanceRequest struct {
	ServiceName string            `json:"serviceName"`
	GroupName   string            `json:"groupName"`
	IP          string            `json:"ip"`
	Port        int               `json:"port"`
	ClusterName string            `json:"clusterName"`
	Healthy     bool              `json:"healthy"`
	Enabled     bool              `json:"enabled"`
	Ephemeral   bool              `json:"ephemeral"`
	Metadata    map[string]string `json:"metadata"`
}

func NewClient(baseURL, namespace string, cli *http.Client) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		namespace: strings.TrimSpace(namespace),
		cli:       cli,
	}
}

func (c *Client) RegisterInstance(ctx context.Context, req RegisterInstanceRequest) error {
	form := c.baseForm(req.ServiceName, req.GroupName)
	form.Set("ip", req.IP)
	form.Set("port", strconv.Itoa(req.Port))
	form.Set("healthy", strconv.FormatBool(req.Healthy))
	form.Set("enabled", strconv.FormatBool(req.Enabled))
	form.Set("ephemeral", strconv.FormatBool(req.Ephemeral))
	if req.ClusterName != "" {
		form.Set("clusterName", req.ClusterName)
	}
	if len(req.Metadata) > 0 {
		raw, _ := json.Marshal(req.Metadata)
		form.Set("metadata", string(raw))
	}
	return c.doWithFallback(ctx, http.MethodPost, "/nacos/v2/ns/instance", "/nacos/v1/ns/instance", form)
}

func (c *Client) DeregisterInstance(ctx context.Context, req RegisterInstanceRequest) error {
	form := c.baseForm(req.ServiceName, req.GroupName)
	form.Set("ip", req.IP)
	form.Set("port", strconv.Itoa(req.Port))
	form.Set("ephemeral", strconv.FormatBool(req.Ephemeral))
	if req.ClusterName != "" {
		form.Set("clusterName", req.ClusterName)
	}
	return c.doWithFallback(ctx, http.MethodDelete, "/nacos/v2/ns/instance", "/nacos/v1/ns/instance", form)
}

func (c *Client) ListInstances(ctx context.Context, serviceName, groupName string, healthyOnly bool) ([]Instance, error) {
	form := c.baseForm(serviceName, groupName)
	form.Set("healthyOnly", strconv.FormatBool(healthyOnly))

	body, err := c.readWithFallback(ctx, "/nacos/v2/ns/instance/list", "/nacos/v1/ns/instance/list", form)
	if err != nil {
		return nil, err
	}
	instances, err := decodeInstanceList(body)
	if err != nil {
		return nil, err
	}
	for i := range instances {
		if instances[i].ServiceName == "" {
			instances[i].ServiceName = serviceName
		}
		if instances[i].GroupName == "" {
			instances[i].GroupName = groupName
		}
	}
	return instances, nil
}

func (c *Client) baseForm(serviceName, groupName string) url.Values {
	form := url.Values{}
	form.Set("serviceName", serviceName)
	if strings.TrimSpace(groupName) != "" {
		form.Set("groupName", groupName)
	}
	if c.namespace != "" {
		form.Set("namespaceId", c.namespace)
	}
	return form
}

func (c *Client) doWithFallback(ctx context.Context, v2Method, v2Path, v1Path string, form url.Values) error {
	if err := c.do(ctx, v2Method, v2Path, form); err == nil {
		return nil
	}
	return c.do(ctx, v2Method, v1Path, form)
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path+"?"+form.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("nacos %s %s -> %d: %s", method, path, resp.StatusCode, string(raw))
	}
	return nil
}

func (c *Client) readWithFallback(ctx context.Context, v2Path, v1Path string, form url.Values) ([]byte, error) {
	body, err := c.read(ctx, v2Path, form)
	if err == nil {
		return body, nil
	}
	return c.read(ctx, v1Path, form)
}

func (c *Client) read(ctx context.Context, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+form.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("nacos GET %s -> %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw, nil
}

func decodeInstanceList(body []byte) ([]Instance, error) {
	type v1Resp struct {
		Hosts []Instance `json:"hosts"`
	}
	var outV1 v1Resp
	if err := json.Unmarshal(body, &outV1); err == nil && len(outV1.Hosts) > 0 {
		return outV1.Hosts, nil
	}

	type v2Resp struct {
		Data struct {
			Hosts []Instance `json:"hosts"`
		} `json:"data"`
	}
	var outV2 v2Resp
	if err := json.Unmarshal(body, &outV2); err == nil && len(outV2.Data.Hosts) > 0 {
		return outV2.Data.Hosts, nil
	}

	return nil, fmt.Errorf("unexpected nacos instance list response: %s", string(body))
}
