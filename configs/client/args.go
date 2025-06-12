package client_args

import "time"

type CliArgs struct {
	ProxyURL string `json:"proxy_url"`
}

type ProxyServerConfig struct {
	Host    string        `json:"host"`
	Timeout time.Duration `json:"timeout"`
}
