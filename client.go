package main

import (
	"encoding/binary"
	"math/rand"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

type Action uint8

//At a later date it would be worth integrating with the IANA package to be fully compliant.
//For now, just implement common protocols
type Protocol uint8

func (a Action) String() string {
	return [...]string{"ActionReceivedAnnounce", "ActionReceivedMapping", "ActionReceivedPeer"}[a]
}

func (p Protocol) String() string {
	return [...]string{"ProtocolAll", "ProtocolTCP", "ProtocolUDP"}[p]
}

//Not the greatest naming, but will do for now.
const (
	ActionReceivedAnnounce = iota
	ActionReceivedMapping
	ActionReceivedPeer
)

const (
	ProtocolAll Protocol = 0
	ProtocolTCP Protocol = 6
	ProtocolUDP Protocol = 17
)

type Event struct {
	Action Action
	Data   interface{}
}

type ClientEpoch struct {
	prevServerTime uint32
	prevClientTime int64
}

type Client struct {
	GatewayAddr net.IP
	Event       chan Event
	Mappings    map[uint16]PortMap
	PeerMappings map[uint16]PeerMap

	conn      *net.UDPConn
	cancelled bool
	epoch     *ClientEpoch
	nonce        []byte
}

type OpDataMap struct {
	Protocol     Protocol
	InternalPort uint16
	ExternalPort uint16 //This is only a suggestion in request. Server ultimately decides.
	ExternalIP   net.IP //Also only a suggestion
}

type OpDataPeer struct {
	OpDataMap
	RemotePort   uint16
	RemoteIP     net.IP
}

//Potentially add in progress bool
type RefreshTime struct {
	Attempt int
	Time int64
}

type PortMap struct {
	OpDataMap
	Active       bool
	Lifetime     uint32
	Refresh      RefreshTime
}

type PeerMap struct {
	PortMap
	RemotePort   uint16
	RemoteIP     net.IP
}

func (data *OpDataMap) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	//Also, marshal is probably the wrong place for this, since it technically doesn't cause an error.
	if data.InternalPort == 0 {
		return nil, ErrPortNotSpecified
	}

	empty := make([]byte, 3)

	internalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(internalPortBytes, data.InternalPort)

	externalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(externalPortBytes, data.ExternalPort)

	addr := make([]byte, 16)
	if data.ExternalIP != nil {
		copy(addr, data.ExternalIP)
	}

	var slices = [][]byte{
		nonce,
		[]byte{byte(data.Protocol)},
		empty,
		internalPortBytes,
		externalPortBytes,
		addr,
	}

	msg = concatCopyPreAllocate(slices)
	return
}

func (data *OpDataMap) unmarshal(msg []byte) (err error) {
	data = &OpDataMap{
		Protocol: Protocol(msg[12]),
		InternalPort: binary.BigEndian.Uint16(msg[16:18]),
		ExternalPort: binary.BigEndian.Uint16(msg[18:20]),
		ExternalIP: net.IP(msg[20:36]),
	}
	return
}

func (data *OpDataPeer) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	//Also, marshal is probably the wrong place for this, since it technically doesn't cause an error.
	if data.InternalPort == 0 {
		return nil, ErrPortNotSpecified
	}

	r1, r2 := make([]byte, 3), make([]byte, 2)

	internalPortBytes, externalPortBytes, remotePortBytes := make([]byte, 2), make([]byte, 2), make([]byte, 2)

	binary.BigEndian.PutUint16(internalPortBytes, data.InternalPort)
	binary.BigEndian.PutUint16(externalPortBytes, data.ExternalPort)
	binary.BigEndian.PutUint16(remotePortBytes, data.RemotePort)

	addr, remoteAddr := make([]byte, 16), make([]byte, 16)
	if data.ExternalIP != nil {
		copy(addr, data.ExternalIP)
	}
	if data.RemoteIP == nil {
		return nil, ErrNoAddress
	}
	copy(remoteAddr, data.RemoteIP)

	var slices = [][]byte{
		nonce,
		[]byte{byte(data.Protocol)},
		r1,
		internalPortBytes,
		externalPortBytes,
		addr,
		remotePortBytes,
		r2,
		remoteAddr,
	}

	msg = concatCopyPreAllocate(slices)
	return
}

func (data *OpDataPeer) unmarshal(msg []byte) (err error) {
	data = &OpDataPeer{
		OpDataMap: OpDataMap{
			Protocol: Protocol(msg[12]),
			InternalPort: binary.BigEndian.Uint16(msg[16:18]),
			ExternalPort: binary.BigEndian.Uint16(msg[18:20]),
			ExternalIP: net.IP(msg[20:36]),
		},
		RemotePort: binary.BigEndian.Uint16(msg[36:38]),
		RemoteIP: net.IP(msg[40:56]),
	}
	return
}

//Need to add support for PCP options later.
func (c *Client) AddPortMapping(protocol Protocol, internalPort, requestedExternalPort uint16, requestedAddr net.IP, lifetime uint32) (err error) {
	//disableChecks is a bool which stops the method from correcting parameters/applying defaults
	if _, exists := c.Mappings[internalPort]; exists {
		//Mapping already exists
		//Should force refresh the mapping. (Using the lifetime parameter if passed.)
		log.Debugf("mapping for port %d exists, refreshing", internalPort)
	}
	//Set minimum lifetime to 2 mins. Less than this is pointless.
	if lifetime < 120 {
		lifetime = 120
	}
	mapData := &OpDataMap{
		Protocol: protocol,
		InternalPort: internalPort,
		ExternalPort: requestedExternalPort,
		ExternalIP: requestedAddr,
	}
	err = c.addMapping(OpCode(OpMap), lifetime, mapData)
	return
}

func (c *Client) AddPeerMapping(protocol Protocol, internalPort, requestedExternalPort, remotePort uint16, requestedAddr, remoteAddr net.IP, lifetime uint32) (err error) {
	//disableChecks is a bool which stops the method from correcting parameters/applying defaults
	if _, exists := c.PeerMappings[internalPort]; exists {
		//Mapping already exists
		//Should force refresh the mapping. (Using the lifetime parameter if passed.)
		log.Debugf("peer mapping for port %d exists, refreshing", internalPort)
	}
	//Set minimum lifetime to 2 mins. Less than this is pointless.
	if lifetime < 120 {
		lifetime = 120
	}
	peerData := &OpDataPeer{
		OpDataMap: OpDataMap{
			Protocol: protocol,
			InternalPort: internalPort,
			ExternalPort: requestedExternalPort,
			ExternalIP: requestedAddr,
		},
		RemotePort: remotePort,
		RemoteIP: remoteAddr,
	}
	err = c.addMapping(OpCode(OpPeer), lifetime, peerData)
	return
}

func (c *Client) DeletePortMapping(internalPort uint16) (err error) {
	//Should send an AddPortMapping request, setting the lifetime to zero
	if m, exists := c.Mappings[internalPort]; exists {
		mapData := &OpDataMap{
			Protocol: m.Protocol,
			InternalPort: m.InternalPort,
			ExternalPort: m.ExternalPort,
			ExternalIP: m.ExternalIP,
		}
		err = c.addMapping(OpMap, 0, mapData)
	}
	//Delete mapping from map
	L:
		for {
			select {
			case event := <-c.Event:
				if event.Action == ActionReceivedMapping {
					m := event.Data.(PortMap)
					if m.InternalPort == internalPort {
						delete(c.Mappings, internalPort)
						break L
					}
				}
			}
			time.Sleep(time.Millisecond)
		}
	return
}

func (c *Client) DeletePeerMapping(internalPort uint16) (err error) {
	//Should send an AddPortMapping request, setting the lifetime to zero
	if m, exists := c.PeerMappings[internalPort]; exists {
		peerData := &OpDataPeer{
			OpDataMap: OpDataMap{
				Protocol: m.Protocol,
				InternalPort: m.InternalPort,
				ExternalPort: m.ExternalPort,
				ExternalIP: m.ExternalIP,
			},
			RemotePort: m.RemotePort,
			RemoteIP: m.RemoteIP,
		}
		err = c.addMapping(OpPeer, 0, peerData)
	}
	//Delete mapping from map
	L:
		for {
			select {
			case event := <-c.Event:
				if event.Action == ActionReceivedPeer {
					m := event.Data.(PeerMap)
					if m.InternalPort == internalPort {
						delete(c.PeerMappings, internalPort)
						break L
					}
				}
			}
			time.Sleep(time.Millisecond)
		}
	return
}

func (c *Client) RefreshPortMapping(internalPort uint16, lifetime uint32) (err error) {
	if m, exists := c.Mappings[internalPort]; exists {
		mapData := &OpDataMap{
			Protocol: m.Protocol,
			InternalPort: m.InternalPort,
			ExternalPort: m.ExternalPort,
			ExternalIP: m.ExternalIP,
		}
		err = c.addMapping(OpCode(OpMap), lifetime, mapData)
	} else {
		err = ErrMappingNotFound
	}
	return
}

func (c *Client) RefreshPeerMapping(internalPort uint16, lifetime uint32) (err error) {
	if m, exists := c.PeerMappings[internalPort]; exists {
		peerData := &OpDataPeer{
			OpDataMap: OpDataMap{
				Protocol: m.Protocol,
				InternalPort: m.InternalPort,
				ExternalPort: m.ExternalPort,
				ExternalIP: m.ExternalIP,
			},
			RemotePort: m.RemotePort,
			RemoteIP: m.RemoteIP,
		}
		err = c.addMapping(OpCode(OpPeer), lifetime, peerData)
	} else {
		err = ErrMappingNotFound
	}
	return
}

func (c *Client) addMapping(op OpCode, lifetime uint32, data interface{}) (err error) {
	var msg []byte
	switch op {
	case OpMap:
		d := data.(*OpDataMap)
		msg, err = d.marshal(c.nonce)
		if err != nil {
			return ErrPeerDataPayload
		}
	case OpPeer:
		d := data.(*OpDataPeer)
		msg, err = d.marshal(c.nonce)
		if err != nil {
			return ErrPeerDataPayload
		}
	}

	addr, err := c.GetInternalAddress()
	if err != nil {
		return ErrNoInternalAddress
	}

	requestData := &RequestPacket{op, lifetime, addr, msg, nil}
	requestDataBytes, err := requestData.marshal()
	if err != nil {
		return ErrRequestDataPayload
	}
	err = c.sendMessage(requestDataBytes)
	if err != nil {
		return ErrNetworkSend
	}
	rt := getRefreshTime(0, lifetime)
	refresh := RefreshTime{
		Attempt: 0,
		Time: rt,
	}

	switch op {
	case OpMap:
		d := data.(*OpDataMap)
		portMap := PortMap{
			OpDataMap: OpDataMap{
				Protocol: d.Protocol,
				InternalPort: d.InternalPort,
				ExternalPort: d.ExternalPort,
				ExternalIP: d.ExternalIP,
			},
			Active: false,
			Lifetime: lifetime,
			Refresh: refresh,
		}
		c.Mappings[d.InternalPort] = portMap
	case OpPeer:
		d := data.(*OpDataPeer)
		peerMap := PeerMap{
			PortMap: PortMap{
				OpDataMap: OpDataMap{
					Protocol: d.Protocol,
					InternalPort: d.InternalPort,
					ExternalPort: d.ExternalPort,
					ExternalIP: d.ExternalIP,
				},
				Active: false,
				Lifetime: lifetime,
				Refresh: refresh,
			},
			RemotePort: d.RemotePort,
			RemoteIP: d.RemoteIP,
		}
		c.PeerMappings[d.InternalPort] = peerMap
	}
	return
}

func (c *Client) epochValid(clientTime int64, serverTime uint32) bool {
	//Function will be used to check whether to trigger mapping renewals and such.
	e := c.epoch
	s := false
	log.Debugf("Prev client time: %d Current client time: %d Prev server time: %d Server time: %d", e.prevClientTime, clientTime, e.prevServerTime, serverTime)
	//Unsure if this timing check logic if spec compliant.
	if e.prevServerTime == 0 {
		//It's the first timestamp, just store it and return true.
		s = true
	} else if ((int64(e.prevServerTime) + (clientTime - e.prevClientTime)) - int64(serverTime)) <= 1 {
		//If in sync, check delta
		clientDelta := clientTime - e.prevClientTime
		serverDelta := serverTime - e.prevServerTime
		if (clientDelta+2 < int64(serverDelta-(serverDelta/16))) || (int64(serverDelta+2) < clientDelta-(clientDelta/16)) {
			s = false
		} else {
			s = true
		}
	} else {
		//PCP server out of sync. Need to trigger refresh
		s = false
	}
	e.prevServerTime = serverTime
	e.prevClientTime = clientTime
	return s
}

func concatCopyPreAllocate(slices [][]byte) []byte {
	var totalLen int
	for _, s := range slices {
		totalLen += len(s)
	}
	tmp := make([]byte, totalLen)
	var i int
	for _, s := range slices {
		i += copy(tmp[i:], s)
	}
	return tmp
}

func getRefreshTime(attempt int, lifetime uint32) int64 {
	//Reset seed on each call to avoid non-pseudorandom intervals over prolonged usage
	rand.Seed(time.Now().UnixNano())
	t := time.Now()
	//See 11.2.1 of RFC6887
	max := t.Unix() + (5 * int64(lifetime)) / (1 << (attempt + 3))
	min := t.Unix() + (int64(lifetime)) / (1 << (attempt + 1))
	var interval int64
	if (max - min) > 0 {
		interval = rand.Int63n(max - min) + min
	}
	if interval < 4 {
		interval = t.Unix() + 4
	}
	log.Debug(max, min, max - min)
	log.Debugf("max - current: %d min - current: %d random int: %d, lifetime: %d", max - t.Unix(), min - t.Unix(), interval - t.Unix(), lifetime)
	log.Debugf("Refresh max: %d Refresh min: %d Time now: %d Interval: %d", max, min, t.Unix(), interval)
	return interval
}
