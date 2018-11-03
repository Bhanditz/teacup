package main

import "encoding/json"

type RpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int64            `json:"id"`
	Method  string           `json:"method,omitempty"`
	Params  *json.RawMessage `json:"params,omitempty"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *RpcError        `json:"error,omitempty"`
}

type RpcError struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    *json.RawMessage `json:"data"`
}
