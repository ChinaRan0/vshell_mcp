package vshell

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tunnel is a tunnel / proxy entry as shown on the /tunnel/list page.
type Tunnel struct {
	Id        int    `json:"Id"`
	ClientId  int    `json:"ClientId"`
	Remark    string `json:"Remark"`
	Mode      string `json:"Mode"` // socks5 | http | tcp | udp
	Port      int    `json:"Port"`
	Target    string `json:"Target"`
	Username  string `json:"Username"`
	Password  string `json:"Password"`
	RunStatus bool   `json:"RunStatus"` // 隧道是否启用
	IsConnect bool   `json:"IsConnect"` // 客户端是否在线
}

// TunnelListResult is the payload of POST /tunnel/list.
type TunnelListResult struct {
	Items []Tunnel `json:"items"`
	Total int      `json:"total"`
}

// AddTunnelRequest carries the fields for creating / editing a tunnel.
// Mode must be one of "socks5", "http", "tcp", "udp". For tcp/udp the
// Target (ip:port) is required; for socks5/http Username/Password are used.
type AddTunnelRequest struct {
	ClientId int    `json:"ClientId"`
	Mode     string `json:"Mode"`
	Port     int    `json:"Port"`
	Target   string `json:"Target,omitempty"`
	Username string `json:"Username,omitempty"`
	Password string `json:"Password,omitempty"`
	Remark   string `json:"Remark,omitempty"`
}

// ListTunnels returns all tunnel/proxy entries.
func (c *Client) ListTunnels(ctx context.Context) ([]Tunnel, error) {
	raw, err := c.Call(ctx, "/tunnel/list", map[string]any{"page": 1, "pageSize": 9999})
	if err != nil {
		return nil, err
	}
	var res TunnelListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse tunnel/list: %w", err)
	}
	return res.Items, nil
}

// AddTunnel creates a new tunnel.
func (c *Client) AddTunnel(ctx context.Context, req AddTunnelRequest) error {
	if req.Mode == "" {
		req.Mode = "socks5"
	}
	_, err := c.Call(ctx, "/tunnel/add", req)
	return err
}

// EditTunnel updates an existing tunnel.
func (c *Client) EditTunnel(ctx context.Context, id int, req AddTunnelRequest) error {
	body := map[string]any{"Id": id, "ClientId": req.ClientId, "Mode": req.Mode, "Port": req.Port}
	if req.Target != "" {
		body["Target"] = req.Target
	}
	if req.Username != "" {
		body["Username"] = req.Username
	}
	if req.Password != "" {
		body["Password"] = req.Password
	}
	if req.Remark != "" {
		body["Remark"] = req.Remark
	}
	_, err := c.Call(ctx, "/tunnel/edit", body)
	return err
}

// SetTunnelRemark updates a tunnel's remark.
func (c *Client) SetTunnelRemark(ctx context.Context, id int, remark string) error {
	_, err := c.Call(ctx, "/tunnel/editremark", map[string]any{"id": id, "remark": remark})
	return err
}

// DeleteTunnel removes a single tunnel.
func (c *Client) DeleteTunnel(ctx context.Context, id int) error {
	_, err := c.Call(ctx, "/tunnel/del", map[string]any{"id": id})
	return err
}

// DeleteTunnels removes multiple tunnels by id.
func (c *Client) DeleteTunnels(ctx context.Context, ids []int) error {
	_, err := c.Call(ctx, "/tunnel/dellist", map[string]any{"id": ids})
	return err
}

// StartTunnel enables a tunnel.
func (c *Client) StartTunnel(ctx context.Context, id int) error {
	_, err := c.Call(ctx, "/tunnel/start", map[string]any{"id": id})
	return err
}

// StopTunnel disables a tunnel.
func (c *Client) StopTunnel(ctx context.Context, id int) error {
	_, err := c.Call(ctx, "/tunnel/stop", map[string]any{"id": id})
	return err
}
