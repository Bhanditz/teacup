package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pkg/errors"
)

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
	Message string           `json:"string"`
	Data    *json.RawMessage `json:"data"`
}

type RpcCode int64

const (
	RpcCodeParseError     RpcCode = -32700
	RpcCodeInvalidRequest RpcCode = -32600
	RpcCodeMethodNotFound RpcCode = -32601
	RpcCodeInvalidParams  RpcCode = -32602
	RpcCodeInternalError  RpcCode = -32603
)

type ProxyConnectParams struct {
	// Address of the TCP endpoint to connect to
	Address string `json:"address"`
}

type ProxyConnectResult struct {
	OK bool `json:"ok"`
}

func handleConn(clientConn net.Conn) {
	clientR := bufio.NewReader(clientConn)
	clientW := bufio.NewWriter(clientConn)
	defer clientConn.Close()

	clientIncoming := make(chan string)
	go func() {
		scanner := bufio.NewScanner(clientR)
		for scanner.Scan() {
			line := scanner.Text()
			clientIncoming <- line
		}
	}()

	var proxyConnectLine string
	select {
	case proxyConnectLine = <-clientIncoming:
		// good!
	case <-time.After(1 * time.Second):
		log.Printf("Timed out waiting for Proxy.Connect")
		return
	}

	var connectReq RpcMessage
	var serverConn net.Conn
	var serverAddress string
	var serverR *bufio.Reader
	var serverW *bufio.Writer
	{
		err := json.Unmarshal([]byte(proxyConnectLine), &connectReq)
		if err != nil {
			log.Printf("While unmarshalling Proxy.Connect message %+v", err)
			return
		}

		if connectReq.JSONRPC != "2.0" {
			log.Printf("Expected request to have json-rpc: 2.0, but got %q", connectReq.JSONRPC)
			return
		}

		replyError := func(errorCode RpcCode, errorMessage string) {
			var msg = RpcMessage{
				JSONRPC: "2.0",
				ID:      connectReq.ID,
				Error: &RpcError{
					Code:    int64(errorCode),
					Message: errorMessage,
				},
			}

			payload, err := json.Marshal(msg)
			must(err)

			_, err = clientW.Write(payload)
			if err != nil {
				log.Printf("Could not write error to client: %+v", err)
			}

			err = clientW.Flush()
			if err != nil {
				log.Printf("Could not flush to client: %+v", err)
			}
		}

		if connectReq.Method != "Proxy.Connect" {
			errMsg := fmt.Sprintf("Expected first call to be Proxy.Connect but was %q", connectReq.Method)
			replyError(RpcCodeInvalidRequest, errMsg)
			log.Print(errMsg)
			return
		}

		var params ProxyConnectParams
		err = json.Unmarshal(*connectReq.Params, &params)
		if err != nil {
			errMsg := fmt.Sprintf("While unmarshalling Proxy.Connect params %+v", err)
			replyError(RpcCodeInvalidParams, errMsg)
			log.Print(errMsg)
			return
		}
		serverAddress = params.Address

		serverConn, err = net.DialTimeout("tcp", serverAddress, 1*time.Second)
		if err != nil {
			errMsg := fmt.Sprintf("While connecting to %s: %+v", serverAddress, err)
			replyError(RpcCodeInternalError, errMsg)
			log.Printf(errMsg)
			return
		}
		defer serverConn.Close()

		serverR = bufio.NewReader(serverConn)
		serverW = bufio.NewWriter(serverConn)

		var result = ProxyConnectResult{
			OK: true,
		}
		resultPayload, err := json.Marshal(result)
		must(err)
		resultPayloadRaw := json.RawMessage(resultPayload)

		var connectRes = RpcMessage{
			JSONRPC: "2.0",
			ID:      connectReq.ID,
			Result:  &resultPayloadRaw,
		}

		connectResPayload, err := json.Marshal(connectRes)
		must(err)

		_, err = clientW.Write(connectResPayload)
		if err != nil {
			log.Printf("While writing Proxy.Connect response: %+v", err)
		}

		err = clientW.Flush()
		if err != nil {
			log.Printf("While writing Proxy.Connect response: %+v", err)
		}
	}

	serverIncoming := make(chan string)
	go func() {
		scanner := bufio.NewScanner(serverR)
		for scanner.Scan() {
			line := scanner.Text()
			serverIncoming <- line
		}
	}()

	sendLine := func(w *bufio.Writer, line string) error {
		var err error
		_, err = w.WriteString(line)
		if err != nil {
			return errors.WithStack(err)
		}
		err = w.WriteByte('\n')
		if err != nil {
			return errors.WithStack(err)
		}
		err = w.Flush()
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}

	log.Printf("Now relaying...")
	for {
		var err error

		select {
		case serverMsg := <-serverIncoming:
			log.Printf("← %s", serverMsg)
			err = sendLine(clientW, serverMsg)
		case clientMsg := <-clientIncoming:
			log.Printf("→ %s", clientMsg)
			err = sendLine(serverW, clientMsg)
		}

		if err != nil {
			log.Printf("%+v", err)
			return
		}
	}
}
