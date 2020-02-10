package main

import (
	"encoding/binary"
	"net"

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

	conn      *net.UDPConn
	cancelled bool
	epoch     *ClientEpoch
	nonce        []byte
}

type OpDataMap struct {
	protocol     Protocol
	internalPort uint16
	externalPort uint16 //This is only a suggestion in request. Server ultimately decides.
	externalIP   net.IP //Also only a suggestion
}

type PortMap struct {
	protocol     Protocol
	internalPort uint16
	externalPort uint16
	externalIP   net.IP
	active       bool
	lifetime     uint32
	expireTime   int64
}

func (data *OpDataMap) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	if data.internalPort == 0 {
		return nil, ErrPortNotSpecified
	}

	empty := make([]byte, 3)

	internalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(internalPortBytes, data.internalPort)

	externalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(externalPortBytes, data.externalPort)

	addr := make([]byte, 16)
	if data.externalIP != nil {
		copy(addr, data.externalIP)
	}

	var slices = [][]byte{
		nonce,
		[]byte{byte(data.protocol)},
		empty,
		internalPortBytes,
		externalPortBytes,
		addr,
	}

	msg = concatCopyPreAllocate(slices)
	return
}

func (data *OpDataMap) unmarshal(msg []byte) (err error) {
	data.protocol = Protocol(msg[12])
	data.internalPort = binary.BigEndian.Uint16(msg[16:18])
	data.externalPort = binary.BigEndian.Uint16(msg[18:20])
	data.externalIP = net.IP(msg[20:36])
	return
}

func (c *Client) RefreshPortMapping(internalPort uint16, lifetime uint32) (err error) {
	//As currently implemented, if this is <=600, the server is going to be spammed with
	//requests. This will be resolved when proper refresh timing is implemented.
	if lifetime == 0 {
		lifetime = DefaultLifetimeSeconds
	}
	if m, exists := c.Mappings[internalPort]; exists {
		err = c.AddPortMapping(m.protocol, m.internalPort, m.externalPort, m.externalIP, lifetime)
	} else {
		err = ErrMappingNotFound
	}
	return
}

//Need to add support for PCP options later.
func (c *Client) AddPortMapping(protocol Protocol, internalPort, requestedExternalPort uint16, requestedAddr net.IP, lifetime uint32) (err error) {
	//Need to check mapping does not already exist. Refresh if it does.
	if _, exists := c.Mappings[internalPort]; exists {
		//Mapping already exists
		//Should force refresh the mapping. (Using the lifetime parameter if passed.)
		log.Debugf("mapping for port %d exists, refreshing", internalPort)
	}
	mapData := &OpDataMap{protocol, internalPort, requestedExternalPort, requestedAddr}
	mapDataBytes, err := mapData.marshal(c.nonce)
	if err != nil {
		return ErrMapDataPayload
	}
	addr, err := GetInternalAddress()
	if err != nil {
		return ErrNoInternalAddress
	}
	requestData := &RequestPacket{OpCode(OpMap), lifetime, addr, mapDataBytes, nil}
	requestDataBytes, err := requestData.marshal()
	if err != nil {
		return ErrRequestDataPayload
	}
	err = c.sendMessage(requestDataBytes)
	if err != nil {
		return ErrNetworkSend
	}
	//Add provisional mapping. Response will set actual port, active and expiryTime
	mapping := PortMap{protocol, internalPort, requestedExternalPort, requestedAddr, false, lifetime, 0}
	c.Mappings[internalPort] = mapping
	return
}

func (c *Client) DeletePortMapping(internalPort uint16) (err error) {
	//Should send an AddPortMapping request, reusing the nonce, but setting the lifetime to zero
	//Mapping should set active = false as opposed to deleting. See section 15 of rfc6887 wrt
	//allowing clients with same nonce to reclaim previously deleted mappings (8th paragraph)
	return
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
