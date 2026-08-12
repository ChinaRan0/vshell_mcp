package vshell

import (
	"context"
	"encoding/json"
	"fmt"
)

// Listener is a C2 listener as shown on the /listener/list page.
// NOTE: deletion of listeners is intentionally NOT supported (per requirements),
// so there is no Delete/DeleteMany method here.
type Listener struct {
	Id               int    `json:"Id"`
	Mode             string `json:"Mode"` // tcp | kcp | ws | dns | doh | dot | oss
	Remark           string `json:"Remark"`
	ListenAddr       string `json:"ListenAddr"`
	ConnectAddr      string `json:"ConnectAddr"`
	WsConnectAddr    string `json:"WsConnectAddr"`
	DNSDomain        string `json:"DNSDomain"`
	PublicDNS        string `json:"PublicDNS"`
	DisconnectTimeout int   `json:"DisconnectTimeout"`
	PingInterval     int    `json:"PingInterval"`
	Vkey             string `json:"Vkey"`
	EncryptSalt      string `json:"EncryptSalt"`
	OssUrl           string `json:"OssUrl"`
	MaxDNSsize       int    `json:"MaxDNSsize"`
	Status           bool   `json:"Status"`
}

// ListenerListResult is the payload of POST /listener/list.
type ListenerListResult struct {
	Items []Listener `json:"items"`
	Total int        `json:"total"`
}

// AddListenerRequest carries the fields for creating a listener. Mode is one
// of "tcp", "kcp", "ws", "dns", "doh", "dot", "oss". Mode-specific addresses:
// tcp/kcp -> ConnectAddr; ws -> WsConnectAddr; dns/doh/dot -> DNSDomain+PublicDNS;
// oss -> OssUrl.
type AddListenerRequest struct {
	ClientId          int    `json:"ClientId"`
	Mode              string `json:"Mode"`
	Remark            string `json:"Remark,omitempty"`
	ListenAddr        string `json:"ListenAddr"`
	ConnectAddr       string `json:"ConnectAddr,omitempty"`
	WsConnectAddr     string `json:"WsConnectAddr,omitempty"`
	DNSDomain         string `json:"DNSDomain,omitempty"`
	PublicDNS         string `json:"PublicDNS,omitempty"`
	DisconnectTimeout int    `json:"DisconnectTimeout"`
	PingInterval      int    `json:"PingInterval"`
	Vkey              string `json:"Vkey"`
	EncryptSalt       string `json:"EncryptSalt"`
	OssUrl            string `json:"OssUrl,omitempty"`
}

// ListListeners returns all listeners. status filters by running status when
// non-nil; mode filters by listener mode when non-empty.
func (c *Client) ListListeners(ctx context.Context, status *bool, mode string) ([]Listener, error) {
	body := map[string]any{"page": 1, "pageSize": 9999}
	if status != nil {
		body["status"] = *status
	}
	if mode != "" {
		body["mode"] = mode
	}
	raw, err := c.Call(ctx, "/listener/list", body)
	if err != nil {
		return nil, err
	}
	var res ListenerListResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse listener/list: %w", err)
	}
	return res.Items, nil
}

// AddListener creates a new listener. (Deletion is intentionally not exposed.)
func (c *Client) AddListener(ctx context.Context, req AddListenerRequest) error {
	if req.Mode == "" {
		req.Mode = "tcp"
	}
	if req.DisconnectTimeout == 0 {
		req.DisconnectTimeout = 30
	}
	if req.PingInterval == 0 {
		req.PingInterval = 10
	}
	_, err := c.Call(ctx, "/listener/add", req)
	return err
}
