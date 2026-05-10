# 🧊 IceScream Protocol

Lightweight peer-to-peer communication protocol with tui written in Go.

```text
╔══════════════════════════════════════════════════════════════════╗
║  [IceScream] alice@:9900                        peers: 2         ║
╠══════════════════════════════════════════════════════════════════╣
║ [14:32:01] -!- bob (:9901) joined the network                    ║
║ [14:32:05] <bob> hey everyone                                    ║
║ [14:32:08] <alice> welcome bob!                                  ║
║ [14:32:10] *upload* <bob> shared 'config.json' (1.2kb)           ║
╠══════════════════════════════════════════════════════════════════╣
║ > _                                                              ║
╚══════════════════════════════════════════════════════════════════╝
```

---

# Features

* ⚡ Lightweight TCP-based P2P networking
* 💬 Real-time chat between peers
* 📁 File transfer support
* 🖥️ Remote command execution
* 🧭 Interactive terminal UI (TUI)
* 🔌 Dynamic peer connections
* 📡 Ping/latency monitoring

---


# Build

```bash
go mod tidy
go build -o icescream .
```

---

# Quick Start

## Start the first node

```bash
./icescream -listen :9900 -nick alice
```

## Connect a second node

```bash
./icescream -listen :9901 -nick bob -connect localhost:9900
```

---

# Commands

| Command                | Description                       |
| ---------------------- | --------------------------------- |
| `/connect <host:port>` | Connect to a peer                 |
| `/peers`               | List active peers                 |
| `/ping`                | Ping all peers and display RTT    |
| `/quit`                | Shutdown the node                 |
| `/help`                | Show available commands           |
| `:chat <message>`      | Send a chat message               |
| `:upload <file>`       | Upload a file to connected peers  |
| `:exec <command>`      | Execute a remote command on peers |
| `PgUp / PgDn`          | Scroll message history            |
| `Ctrl+C` / `Esc`       | Exit the application              |

> Any text without a command prefix is automatically treated as `:chat`.

---

# Architecture

```text
main.go       — CLI flags and application entrypoint
protocol.go   — message types and JSON-line protocol
node.go       — TCP networking, peer management, NodeOutput interface
handlers.go   — message handlers by protocol type
tui.go        — tui using go + tcell
```

---

# Protocol Overview

IceScream uses a simple TCP-based peer-to-peer architecture.

* Nodes communicate using newline-delimited JSON messages
* Peers can dynamically join the network
* Messages are broadcast across connected peers
* Designed for experimentation, local networks, and distributed tooling

Example message:

```json
{
  "type": "chat",
  "nick": "alice",
  "body": "hello world"
}
```

---

# Security Notes

⚠️ This project is experimental and not production-ready.

Current limitations:

* No TLS encryption
* No authentication layer
* No peer verification
* `:exec` runs real system commands

Before using in production:

* Add TLS support
* Implement authentication/authorization
* Restrict or sandbox remote execution
* Add peer trust validation

---

# Roadmap

* [ ] TLS support
* [ ] Peer authentication
* [ ] NAT traversal
* [ ] Gossip-based peer discovery
* [ ] End-to-end encryption
* [ ] Binary protocol support
* [ ] Plugin system

---

#### Based in IRSSI
