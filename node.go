package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
)

const ProtocolVersion = "0.1.0"

type NodeOutput interface {
	WriteSystem(text string)
	WriteError(text string)
	WriteJoin(nick, addr string)
	WritePart(nick, addr string)
	WriteChat(nick, text string, self bool)
	WriteUpload(from, filename string, size int64)
	WriteExec(from, cmd, reqID string)
	WriteExecReply(from, output, errStr string)
}

type fallbackOutput struct{}

func (f *fallbackOutput) WriteSystem(t string)                  { fmt.Println("[*]", t) }
func (f *fallbackOutput) WriteError(t string)                   { fmt.Println("[!]", t) }
func (f *fallbackOutput) WriteJoin(nick, addr string)           { fmt.Printf("[+] %s (%s)\n", nick, addr) }
func (f *fallbackOutput) WritePart(nick, addr string)           { fmt.Printf("[-] %s (%s)\n", nick, addr) }
func (f *fallbackOutput) WriteChat(nick, text string, _ bool)   { fmt.Printf("<%s> %s\n", nick, text) }
func (f *fallbackOutput) WriteUpload(from, fn string, sz int64) { fmt.Printf("[upload] %s: %s (%d)\n", from, fn, sz) }
func (f *fallbackOutput) WriteExec(from, cmd, req string)       { fmt.Printf("[exec] %s: %s (%s)\n", from, cmd, req) }
func (f *fallbackOutput) WriteExecReply(from, out, e string)    { fmt.Printf("[exec-reply] %s: %s %s\n", from, out, e) }

type Node struct {
	listenAddr string
	nickname   string

	peers   map[string]*Peer
	peersMu sync.RWMutex

	listener net.Listener
	quit     chan struct{}

	out   NodeOutput
	outMu sync.Mutex
}

type Peer struct {
	addr   string
	conn   net.Conn
	writer *bufio.Writer
	mu     sync.Mutex
}

func NewNode(listenAddr, nickname string) *Node {
	return &Node{
		listenAddr: listenAddr,
		nickname:   nickname,
		peers:      make(map[string]*Peer),
		quit:       make(chan struct{}),
		out:        &fallbackOutput{},
	}
}

func (n *Node) SetOutput(out NodeOutput) {
	n.outMu.Lock()
	defer n.outMu.Unlock()
	n.out = out
}

func (n *Node) emit() NodeOutput {
	n.outMu.Lock()
	defer n.outMu.Unlock()
	return n.out
}

func (n *Node) Start() error {
	ln, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", n.listenAddr, err)
	}
	n.listener = ln
	go n.acceptLoop()
	return nil
}

func (n *Node) acceptLoop() {
	for {
		conn, err := n.listener.Accept()
		if err != nil {
			select {
			case <-n.quit:
				return
			default:
				n.emit().WriteError(fmt.Sprintf("accept: %v", err))
				continue
			}
		}
		go n.handleIncoming(conn)
	}
}

func (n *Node) handleIncoming(conn net.Conn) {
	peer := &Peer{
		addr:   conn.RemoteAddr().String(),
		conn:   conn,
		writer: bufio.NewWriter(conn),
	}
	n.sendHandshake(peer)
	n.registerPeer(peer)
	n.readLoop(peer)
}

func (n *Node) Connect(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connect %s: %w", addr, err)
	}
	peer := &Peer{
		addr:   addr,
		conn:   conn,
		writer: bufio.NewWriter(conn),
	}
	n.sendHandshake(peer)
	n.registerPeer(peer)
	go n.readLoop(peer)
	n.emit().WriteSystem(fmt.Sprintf("Connected to %s", addr))
	return nil
}

func (n *Node) Quit() {
	select {
	case <-n.quit:
		return
	default:
	}
	close(n.quit)

	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	for _, p := range n.peers {
		msg := NewMessage(MsgBye, n.nickname, n.listenAddr, nil)
		_ = n.sendTo(p, msg)
		p.conn.Close()
	}
	if n.listener != nil {
		n.listener.Close()
	}
}

func (n *Node) registerPeer(p *Peer) {
	n.peersMu.Lock()
	defer n.peersMu.Unlock()
	n.peers[p.addr] = p
}

func (n *Node) removePeer(p *Peer) {
	n.peersMu.Lock()
	delete(n.peers, p.addr)
	n.peersMu.Unlock()
	p.conn.Close()
	n.emit().WritePart(p.addr, p.addr)
}

func (n *Node) knownPeerAddrs() []string {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	addrs := make([]string, 0, len(n.peers))
	for addr := range n.peers {
		addrs = append(addrs, addr)
	}
	return addrs
}

func (n *Node) peerCount() int {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	return len(n.peers)
}

func (n *Node) sendTo(p *Peer, msg *Message) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err = p.writer.Write(data); err != nil {
		return err
	}
	return p.writer.Flush()
}

func (n *Node) Broadcast(msg *Message) {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	for _, p := range n.peers {
		if err := n.sendTo(p, msg); err != nil {
			n.emit().WriteError(fmt.Sprintf("broadcast → %s: %v", p.addr, err))
		}
	}
}

func (n *Node) sendHandshake(p *Peer) {
	payload := HandshakePayload{
		Nickname:   n.nickname,
		ListenAddr: n.listenAddr,
		KnownPeers: n.knownPeerAddrs(),
	}
	msg := NewMessage(MsgHandshake, n.nickname, n.listenAddr, payload)
	if err := n.sendTo(p, msg); err != nil {
		n.emit().WriteError(fmt.Sprintf("handshake → %s: %v", p.addr, err))
	}
}

func (n *Node) readLoop(p *Peer) {
	scanner := bufio.NewScanner(p.conn)
	buf := make([]byte, 10*1024*1024)
	scanner.Buffer(buf, cap(buf))

	for scanner.Scan() {
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		msg, err := DecodeMessage(raw)
		if err != nil {
			n.emit().WriteError(fmt.Sprintf("decode of %s: %v", p.addr, err))
			continue
		}
		n.handleMessage(p, msg)
	}
	n.removePeer(p)
}
