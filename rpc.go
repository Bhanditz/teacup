package main

import "encoding/json"

type RpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int64            `json:"id"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params"`
	Result  *json.RawMessage `json:"result"`
	Error   *RpcError        `json:"error"`
}

type RpcError struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    *json.RawMessage `json:"data"`
}
