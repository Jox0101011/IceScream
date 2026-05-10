package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	listenAddr := flag.String("listen", ":9900", "Address to listen connections (ex: :9900)")
	connectTo  := flag.String("connect", "", "Peer to connect in start (ex: 192.168.1.10:9900)")
	nickname   := flag.String("nick", "noName", "Your nickname in the IceScream protocol")
	flag.Parse()

	fmt.Println(`
  ___          ____                            
 |_ _|___  ___/ ___|  ___ _ __ ___  __ _ _ __ ___  
  | |/ __\/ _ \___ \ / __| '__/ _ \/ _  | '_ ' _ \ 
  | | (__|  __/___) | (__| | |  __/ (_| | | | | | |
 |___\___\___|____/ \___|_|  \___|\__,_|_| |_| |_|
                                          v0.1.2-alpha
  Protocol P2P IceScream — ice and scream on net.
`)

	node := NewNode(*listenAddr, *nickname)

	if err := node.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERRO] Failed to started node: %v\n", err)
		os.Exit(1)
	}

	if *connectTo != "" {
		if err := node.Connect(*connectTo); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to connect in %s: %v\n", *connectTo, err)
		}
	}

	node.RunCLI()
}
