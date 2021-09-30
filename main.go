package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	noise "github.com/libp2p/go-libp2p-noise"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const protocolID = "/example/1.0.0"
const discoveryNamespace = "example"

var node host.Host

func main() {
	// Add -peer-address flag
	peerAddr := flag.String("peer-address", "", "peer address")
	flag.Parse()
	var err error

	// Create the libp2p node.
	//
	// Note that we are explicitly passing the listen address and restricting it to IPv4 over the
	// loopback interface (127.0.0.1).
	//
	// Setting the TCP port as 0 makes libp2p choose an available port for us.
	// You could, of course, specify one if you like.
	//node, err = libp2p.New(context.Background(), libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	node, err = libp2p.New(
		context.Background(),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
	)
	if err != nil {
		panic(err)
	}
	defer node.Close()

	// Print this node's addresses and ID
	fmt.Println("Addresses:", node.Addrs())
	fmt.Println("ID:", node.ID())
	for _, multiAddr := range node.Addrs() {
		fmt.Println("Full Multi Addr: ", multiAddr.String()+"/p2p/"+node.ID().String())
	}

	// Setup a stream handler.
	//
	// This gets called every time a peer connects and opens a stream to this node.
	node.SetStreamHandler(protocolID, func(s network.Stream) {
		go writeCounter(s)
		go readCounter(s)
	})

	// Setup peer discovery.
	discoveryService := mdns.NewMdnsService(
		node,
		discoveryNamespace,
	)
	defer discoveryService.Close()

	discoveryService.RegisterNotifee(&discoveryNotifee{h: node})

	// If we received a peer address, we should connect to it.
	if *peerAddr != "" {
		// Parse the multiaddr string.
		peerMA, err := multiaddr.NewMultiaddr(*peerAddr)
		if err != nil {
			panic(err)
		}
		peerAddrInfo, err := peer.AddrInfoFromP2pAddr(peerMA)
		if err != nil {
			panic(err)
		}

		connectToPeer(*peerAddrInfo)
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGKILL, syscall.SIGINT)
	<-sigCh
}

func connectToPeer(peerAddrInfo peer.AddrInfo) error {
	// Connect to the node at the given address.
	if err := node.Connect(context.Background(), peerAddrInfo); err != nil {
		return err
	}
	fmt.Println("Connected to", peerAddrInfo.String())

	// Open a stream with the given peer.
	s, err := node.NewStream(context.Background(), peerAddrInfo.ID, protocolID)
	if err != nil {
		return err
	}

	// Start the write and read threads.
	go writeCounter(s)
	go readCounter(s)
	return nil
}

func writeCounter(s network.Stream) {
	var counter uint64

	for {
		<-time.After(time.Second)
		counter++

		err := binary.Write(s, binary.BigEndian, counter)
		if err != nil {
			fmt.Printf("Error writing to %s: %+v\n", s.ID(), err)

			peerAddr := peer.AddrInfo{
				ID:    s.Conn().RemotePeer(),
				Addrs: []multiaddr.Multiaddr{s.Conn().RemoteMultiaddr()},
			}

			fmt.Printf("Trying to reconnect: %+v\n", peerAddr)

			// try to reconnect
			connectToPeer(peerAddr)

			return
			//panic(err)
		}
	}
}

func readCounter(s network.Stream) {
	for {
		var counter uint64

		err := binary.Read(s, binary.BigEndian, &counter)
		if err != nil {
			fmt.Printf("Error reading from %s: %+v\n", s.ID(), err)
			return
			//panic(err)
		}

		fmt.Printf("Received %d from %s\n", counter, s.ID())
	}
}

type discoveryNotifee struct {
	h host.Host
}

func (n *discoveryNotifee) HandlePeerFound(peerInfo peer.AddrInfo) {
	fmt.Println("found peer", peerInfo.String())
	if peerInfo.ID != n.h.ID() {
		connectToPeer(peerInfo)
	}
}
