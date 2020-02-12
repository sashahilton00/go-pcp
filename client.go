package main

import (
	"net"
	"time"

	"github.com/jackpal/gateway"
	log "github.com/sirupsen/logrus"
)

type Action uint8

func (a Action) String() string {
	return [...]string{"ActionReceivedAnnounce", "ActionReceivedMapping", "ActionReceivedPeer"}[a]
}

//Not the greatest naming, but will do for now.
const (
	ActionReceivedAnnounce = iota
	ActionReceivedMapping
	ActionReceivedPeer
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
	GatewayAddr  net.IP
	Event        chan Event
	Mappings     map[uint16]PortMap
	PeerMappings map[uint16]PeerMap

	conn      *net.UDPConn
	cancelled bool
	epoch     *ClientEpoch
	nonce     []byte
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
		Protocol:     protocol,
		InternalPort: internalPort,
		ExternalPort: requestedExternalPort,
		ExternalIP:   requestedAddr,
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
			Protocol:     protocol,
			InternalPort: internalPort,
			ExternalPort: requestedExternalPort,
			ExternalIP:   requestedAddr,
		},
		RemotePort: remotePort,
		RemoteIP:   remoteAddr,
	}
	err = c.addMapping(OpCode(OpPeer), lifetime, peerData)
	return
}

func (c *Client) DeletePortMapping(internalPort uint16) (err error) {
	//Should send an AddPortMapping request, setting the lifetime to zero
	if m, exists := c.Mappings[internalPort]; exists {
		mapData := &OpDataMap{
			Protocol:     m.Protocol,
			InternalPort: m.InternalPort,
			ExternalPort: m.ExternalPort,
			ExternalIP:   m.ExternalIP,
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
				Protocol:     m.Protocol,
				InternalPort: m.InternalPort,
				ExternalPort: m.ExternalPort,
				ExternalIP:   m.ExternalIP,
			},
			RemotePort: m.RemotePort,
			RemoteIP:   m.RemoteIP,
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

func (c *Client) GetGatewayAddress() (addr net.IP, err error) {
	addr, err = gateway.DiscoverGateway()
	//Only during testing
	addr = net.ParseIP("127.0.0.1")
	if err != nil {
		return nil, ErrGatewayNotFound
	}
	return addr, nil
}

func (c *Client) GetExternalAddress() (addr net.IP, err error) {
	// Will create a short mapping with PCP server and return the address returned
	// by the server in the response packet. Use UDP/9 (Discard) as short mapping.
	mapData := &OpDataMap{
		Protocol:     ProtocolUDP,
		InternalPort: 9,
		ExternalPort: 0,
		ExternalIP:   net.ParseIP("127.0.0.1"),
	}
	err = c.addMapping(OpMap, 30, mapData)
	if err != nil {
		log.Error(err)
		return nil, err
	}
L:
	for {
		select {
		case event := <-c.Event:
			if event.Action == ActionReceivedMapping {
				m := event.Data.(PortMap)
				if m.InternalPort == 9 {
					addr = m.ExternalIP
					delete(c.Mappings, 9)
					break L
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	return
}

func (c *Client) GetInternalAddress() (addr net.IP, err error) {
	gatewayAddr, err := c.GetGatewayAddress()
	if err != nil {
		return nil, err
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			switch x := addr.(type) {
			case *net.IPNet:
				if x.Contains(gatewayAddr) {
					return x.IP, nil
				}
			}
		}
	}

	return nil, ErrNoInternalAddress
}

func (c *Client) RefreshPortMapping(internalPort uint16, lifetime uint32) (err error) {
	if m, exists := c.Mappings[internalPort]; exists {
		mapData := &OpDataMap{
			Protocol:     m.Protocol,
			InternalPort: m.InternalPort,
			ExternalPort: m.ExternalPort,
			ExternalIP:   m.ExternalIP,
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
				Protocol:     m.Protocol,
				InternalPort: m.InternalPort,
				ExternalPort: m.ExternalPort,
				ExternalIP:   m.ExternalIP,
			},
			RemotePort: m.RemotePort,
			RemoteIP:   m.RemoteIP,
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
		Time:    rt,
	}

	switch op {
	case OpMap:
		d := data.(*OpDataMap)
		portMap := PortMap{
			OpDataMap: OpDataMap{
				Protocol:     d.Protocol,
				InternalPort: d.InternalPort,
				ExternalPort: d.ExternalPort,
				ExternalIP:   d.ExternalIP,
			},
			Active:   false,
			Lifetime: lifetime,
			Refresh:  refresh,
		}
		c.Mappings[d.InternalPort] = portMap
	case OpPeer:
		d := data.(*OpDataPeer)
		peerMap := PeerMap{
			PortMap: PortMap{
				OpDataMap: OpDataMap{
					Protocol:     d.Protocol,
					InternalPort: d.InternalPort,
					ExternalPort: d.ExternalPort,
					ExternalIP:   d.ExternalIP,
				},
				Active:   false,
				Lifetime: lifetime,
				Refresh:  refresh,
			},
			RemotePort: d.RemotePort,
			RemoteIP:   d.RemoteIP,
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
