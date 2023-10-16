//go:build !darwin && !android

package ble

import (
	"fmt"
	"log"
	"sync"
	"time"
	"go.uber.org/zap"

	"berty.tech/weshnet/pkg/proximitytransport"

	"github.com/libp2p/go-libp2p/core/network"
)


type Driver struct {
	protocolCode int
	protocolName string
	defaultAddr  string
}

var local_pid = ""

//var remote_pid = ""

//var send_pid bool = true

// Noop implementation for platform that are not Darwin

func NewDriver(logger *zap.Logger) proximitytransport.ProximityDriver {
	//logger = logger.Named("BLE")
	//logger.Debug("NewDriver()")
	fmt.Println("NewDriver()")

	return &Driver{
		protocolCode: ProtocolCode,
		protocolName: ProtocolName,
		defaultAddr:  DefaultAddr,
	}
}

func BLEHandleFoundPeer(remotePID string) int {

	t, ok := proximitytransport.TransportMap[ProtocolName]
	if !ok {
		fmt.Println("CANT LOAD PROTOCOL NAME")
		return 0
	}
	if t.HandleFoundPeer(remotePID) {
		fmt.Println("Found Peeeeeeeeeeeeeeeer : ", remotePID)

		return 1
	}
	fmt.Println("CANT HANDLE FOUND PEER")
	return 1

}

//export BLEHandleLostPeer
func BLEHandleLostPeer(remotePID string) {
	s := fmt.Sprintf("Lost Peer : %s", remotePID)
	fmt.Println(s)

	t, ok := proximitytransport.TransportMap[ProtocolName]
	if !ok {
		return
	}
	t.HandleLostPeer(remotePID)
}

//export BLEReceiveFromPeer
func BLEReceiveFromPeer(remotePID string, payload []byte) {
	fmt.Println("Receive from Peer : ", string(payload))
	//fmt.Println("Receive from Peer BYTE : ", payload)

	t, ok := proximitytransport.TransportMap[ProtocolName]
	if !ok {
		fmt.Println("CANT LOAD PROTOCOL NAME")
		return
	}
	t.ReceiveFromPeer(remotePID, payload)
}

func (d *Driver) DialPeer(remotePID string) bool {

	fmt.Println("DIAL TO PEER : ", remotePID)

	return true
}

func (d *Driver) SendToPeer(remotePID string, payload []byte) bool {

	var w sync.WaitGroup
	var m sync.Mutex

	time.Sleep(10 * time.Second)

	s := fmt.Sprintf("SEND to Peer : %s   %s", remotePID, string(payload))
	fmt.Println(s)

	//data := strings.TrimSuffix(string(payload), "\n")

	//w.Add(1)

	go protocolBridge.writeData(payload, &w, &m)

	//time.Sleep(20 * time.Second)

	//w.Wait()

	return true
}

func (d *Driver) Start(localPID string) {

	log.Println("LOCAL PID: ", localPID)

	local_pid = localPID

	//protocolBridge.writePID(localPID)

}

func (d *Driver) Stop() {

}

func (d *Driver) CloseConnWithPeer(remotePID string) {
	t, ok := proximitytransport.TransportMap[ProtocolName]
	if !ok {
		fmt.Println("CANT LOAD PROTOCOL NAME")
		return
	}
	t.HandleLostPeer(remotePID)
}

func (d *Driver) ProtocolCode() int {
	return ProtocolCode
}

func (d *Driver) ProtocolName() string {
	return ProtocolName
}

func (d *Driver) DefaultAddr() string {
	return DefaultAddr
}

func copyOfRange(src []byte, from, to int) []byte {
	return append([]byte(nil), src[from:to]...)
}

func HandleStream(s network.Stream) {
	go protocolBridge.handleStream(s)
}
