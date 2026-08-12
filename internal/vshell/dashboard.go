package vshell

import (
	"context"
	"encoding/json"
	"fmt"
)

// ServerConfig describes how this client is connected to v_shell.
type ServerConfig struct {
	BaseURL  string `json:"base_url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Prefix   string `json:"api_prefix"`
}

// Info returns the connection configuration (URL / account / password).
func (c *Client) Info() ServerConfig {
	return ServerConfig{
		BaseURL:  c.cfg.BaseURL,
		Username: c.cfg.Username,
		Password: c.cfg.Password,
		Prefix:   c.cfg.Prefix,
	}
}

// UserInfo is the payload of GET /getUserInfo.
type UserInfo struct {
	RealName string `json:"realName"`
	Username string `json:"username"`
	Desc     string `json:"desc"`
	HomePath string `json:"homePath"`
	UserId   string `json:"userId"`
	Roles    []struct {
		RoleName string `json:"roleName"`
		Value    string `json:"value"`
	} `json:"roles"`
}

// GetUserInfo returns the currently logged-in account details.
func (c *Client) GetUserInfo(ctx context.Context) (*UserInfo, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	url := c.apiURL("/getUserInfo")
	req, err := newGET(ctx, url, tok)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("parse getUserInfo: %w", err)
	}
	if envelope.Code != 0 {
		return nil, &apiError{Code: envelope.Code, Message: envelope.Message}
	}
	var info UserInfo
	if err := json.Unmarshal(envelope.Result, &info); err != nil {
		return nil, fmt.Errorf("parse getUserInfo result: %w", err)
	}
	return &info, nil
}

// Dashboard holds the v_shell server metrics from POST /dashboard/info
// (the data shown on the management console's dashboard).
type Dashboard struct {
	ClientCount        int    `json:"clientCount"`
	ClientOnlineCount  int    `json:"clientOnlineCount"`
	ClientNum          string `json:"clientNum"`
	HostCount          int    `json:"hostCount"`
	ListenerCount      int    `json:"listenerCount"`
	ListenerOnlineCount int   `json:"listenerOnlineCount"`
	Socks5Count        int    `json:"socks5Count"`
	P2PCount           int    `json:"p2pCount"`
	SecretCount        int    `json:"secretCount"`
	HTTPProxyCount     int    `json:"httpProxyCount"`

	CPU           int    `json:"cpu"`
	Disk          int    `json:"disk"`
	VirtualMem    int    `json:"virtual_mem"`
	SwapMem       int    `json:"swap_mem"`
	Load          string `json:"load"`

	IO_Recv int64 `json:"io_recv"`
	IO_Send int64 `json:"io_send"`
	InletFlowCount  int64 `json:"inletFlowCount"`
	ExportFlowCount int64 `json:"exportFlowCount"`

	TCPCount int `json:"tcpCount"`
	UDPCount int `json:"udpCount"`
	TCPC     int `json:"tcpC"`

	Version      string `json:"version"`
	VIP          bool   `json:"vip"`
	LicTime      string `json:"licTime"`
	WebPort      string `json:"web_port"`
	WebBasicAuth bool   `json:"web_basic_auth"`
	LogLevel     string `json:"logLevel"`
	LogPath      string `json:"logPath"`
}

// GetDashboard returns the v_shell server dashboard metrics.
func (c *Client) GetDashboard(ctx context.Context) (*Dashboard, error) {
	raw, err := c.Call(ctx, "/dashboard/info", map[string]any{})
	if err != nil {
		return nil, err
	}
	var db Dashboard
	if err := json.Unmarshal(raw, &db); err != nil {
		return nil, fmt.Errorf("parse dashboard/info: %w", err)
	}
	return &db, nil
}
