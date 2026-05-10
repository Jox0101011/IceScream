package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type MessageType string

const (
	MsgChat      MessageType = "CHAT"       
	MsgUpload    MessageType = "UPLOAD"    
	MsgExec      MessageType = "EXEC" 
	MsgExecReply MessageType = "EXEC_REPLY" 
	MsgHandshake MessageType = "HANDSHAKE" 
	MsgPeerList  MessageType = "PEER_LIST"
	MsgPing      MessageType = "PING"
	MsgPong      MessageType = "PONG"
	MsgBye       MessageType = "BYE"
)

type Message struct {
	Version   string      `json:"version"`
	Type      MessageType `json:"type"`
	From      string      `json:"from"`
	FromAddr  string      `json:"from_addr"`
	Timestamp int64       `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type ChatPayload struct {
	Text string `json:"text"`
}

type UploadPayload struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Data     []byte `json:"data"`
}

type ExecPayload struct {
	Command string `json:"command"`
	ReqID   string `json:"req_id"` 
}

type ExecReplyPayload struct {
	ReqID  string `json:"req_id"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

type HandshakePayload struct {
	Nickname  string   `json:"nickname"`
	ListenAddr string  `json:"listen_addr"`
	KnownPeers []string `json:"known_peers"`
}

type PeerListPayload struct {
	Peers []string `json:"peers"`
}

func NewMessage(msgType MessageType, from, fromAddr string, payload interface{}) *Message {
	return &Message{
		Version:   ProtocolVersion,
		Type:      msgType,
		From:      from,
		FromAddr:  fromAddr,
		Timestamp: time.Now().UnixMilli(),
		Payload:   payload,
	}
}

func (m *Message) Encode() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return append(data, '\n'), nil
}

func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &msg, nil
}

func PayloadAs(raw interface{}, target interface{}) error {
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

/* Welcome to the new era -- hzr */
