package main

import (
	"net"
	"time"

	"github.com/jackpal/gateway"
	log "github.com/sirupsen/logrus"
)

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
		Protocol: ProtocolUDP,
		InternalPort: 9,
		ExternalPort: 0,
		ExternalIP: net.ParseIP("127.0.0.1"),
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
						delete(c.Mappings,9)
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
