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

//Potentially add in progress bool
type RefreshTime struct {
	attempt int
	time int64
}

type PortMap struct {
	protocol     Protocol
	internalPort uint16
	externalPort uint16
	externalIP   net.IP
	active       bool
	lifetime     uint32
	refresh      RefreshTime
}

func (data *OpDataMap) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	//Also, marshal is probably the wrong place for this, since it technically doesn't cause an error.
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
	if m, exists := c.Mappings[internalPort]; exists {
		err = c.AddPortMapping(m.protocol, m.internalPort, m.externalPort, m.externalIP, lifetime, false)
	} else {
		err = ErrMappingNotFound
	}
	return
}

//Need to add support for PCP options later.
func (c *Client) AddPortMapping(protocol Protocol, internalPort, requestedExternalPort uint16, requestedAddr net.IP, lifetime uint32, disableChecks bool) (err error) {
	//disableChecks is a bool which stops the method from correcting parameters/applying defaults
	if _, exists := c.Mappings[internalPort]; exists {
		//Mapping already exists
		//Should force refresh the mapping. (Using the lifetime parameter if passed.)
		log.Debugf("mapping for port %d exists, refreshing", internalPort)
	}
	//Set minimum lifetime to 2 mins. Less than this is pointless.
	if !disableChecks && lifetime < 120 {
		lifetime = 120
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
	//Add provisional mapping. Response will set actual port, active and refresh
	rt := getRefreshTime(0, lifetime)
	refresh := RefreshTime{0, rt}
	mapping := PortMap{protocol, internalPort, requestedExternalPort, requestedAddr, false, lifetime, refresh}
	c.Mappings[internalPort] = mapping
	return
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

func (c *Client) DeletePortMapping(internalPort uint16) (err error) {
	//Should send an AddPortMapping request, reusing the nonce, but setting the lifetime to zero
	//Mapping should set active = false as opposed to deleting. See section 15 of rfc6887 wrt
	//allowing clients with same nonce to reclaim previously deleted mappings (8th paragraph)
	if m, exists := c.Mappings[internalPort]; exists {
		err = c.AddPortMapping(m.protocol, m.internalPort, m.externalPort, m.externalIP, 0, true)
	}
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
