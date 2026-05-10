package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (n *Node) handleMessage(p *Peer, msg *Message) {
	switch msg.Type {
	case MsgHandshake:
		n.onHandshake(p, msg)
	case MsgChat:
		n.onChat(msg)
	case MsgUpload:
		n.onUpload(msg)
	case MsgExec:
		n.onExec(p, msg)
	case MsgExecReply:
		n.onExecReply(msg)
	case MsgPing:
		n.onPing(p, msg)
	case MsgPong:
		rtt := time.Now().UnixMilli() - msg.Timestamp
		n.emit().WriteSystem(fmt.Sprintf("PONG of %s — %dms", msg.From, rtt))
	case MsgBye:
		n.removePeer(p)
	case MsgPeerList:
		n.onPeerList(msg)
	default:
		n.emit().WriteError(fmt.Sprintf("type unknown '%s' of %s", msg.Type, msg.From))
	}
}

func (n *Node) onHandshake(p *Peer, msg *Message) {
	var hs HandshakePayload
	if err := PayloadAs(msg.Payload, &hs); err != nil {
		n.emit().WriteError(fmt.Sprintf("handshake invalid: %v", err))
		return
	}
	if hs.ListenAddr != "" && hs.ListenAddr != p.addr {
		n.peersMu.Lock()
		delete(n.peers, p.addr)
		p.addr = hs.ListenAddr
		n.peers[p.addr] = p
		n.peersMu.Unlock()
	}
	n.emit().WriteJoin(hs.Nickname, hs.ListenAddr)
	go n.discoverPeers(hs.KnownPeers)
}

func (n *Node) onChat(msg *Message) {
	var payload ChatPayload
	if err := PayloadAs(msg.Payload, &payload); err != nil {
		n.emit().WriteError(fmt.Sprintf("chat invalid: %v", err))
		return
	}
	n.emit().WriteChat(msg.From, payload.Text, false)
}

func (n *Node) onUpload(msg *Message) {
	var payload UploadPayload
	if err := PayloadAs(msg.Payload, &payload); err != nil {
		n.emit().WriteError(fmt.Sprintf("upload invalid: %v", err))
		return
	}
	dir := "received"
	if err := os.MkdirAll(dir, 0755); err != nil {
		n.emit().WriteError(fmt.Sprintf("mkdir received: %v", err))
		return
	}
	dest := filepath.Join(dir, filepath.Base(payload.Filename))
	if err := os.WriteFile(dest, payload.Data, 0644); err != nil {
		n.emit().WriteError(fmt.Sprintf("save file: %v", err))
		return
	}
	n.emit().WriteUpload(msg.From, payload.Filename, payload.Size)
}

func (n *Node) onExec(p *Peer, msg *Message) {
	var payload ExecPayload
	if err := PayloadAs(msg.Payload, &payload); err != nil {
		n.emit().WriteError(fmt.Sprintf("exec invalid: %v", err))
		return
	}
	n.emit().WriteExec(msg.From, payload.Command, payload.ReqID)

	parts := strings.Fields(payload.Command)
	if len(parts) == 0 {
		return
	}
	out, execErr := exec.Command(parts[0], parts[1:]...).CombinedOutput() //nolint:gosec

	reply := ExecReplyPayload{ReqID: payload.ReqID, Output: string(out)}
	if execErr != nil {
		reply.Error = execErr.Error()
	}
	replyMsg := NewMessage(MsgExecReply, n.nickname, n.listenAddr, reply)
	if err := n.sendTo(p, replyMsg); err != nil {
		n.emit().WriteError(fmt.Sprintf("send exec-reply: %v", err))
	}
}

func (n *Node) onExecReply(msg *Message) {
	var payload ExecReplyPayload
	if err := PayloadAs(msg.Payload, &payload); err != nil {
		n.emit().WriteError(fmt.Sprintf("exec-reply invalid: %v", err))
		return
	}
	n.emit().WriteExecReply(msg.From, payload.Output, payload.Error)
}

func (n *Node) onPing(p *Peer, msg *Message) {
	pong := NewMessage(MsgPong, n.nickname, n.listenAddr, nil)
	pong.Timestamp = msg.Timestamp
	_ = n.sendTo(p, pong)
}

func (n *Node) onPeerList(msg *Message) {
	var payload PeerListPayload
	if err := PayloadAs(msg.Payload, &payload); err != nil {
		return
	}
	go n.discoverPeers(payload.Peers)
}

func (n *Node) discoverPeers(addrs []string) {
	for _, addr := range addrs {
		if addr == n.listenAddr {
			continue
		}
		n.peersMu.RLock()
		_, known := n.peers[addr]
		n.peersMu.RUnlock()
		if !known {
			if err := n.Connect(addr); err != nil {
				n.emit().WriteSystem(fmt.Sprintf("discover: not connect to %s", addr))
			}
		}
	}
}
