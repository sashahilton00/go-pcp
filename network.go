package main

import (
	"crypto/rand"
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

//Potentially add deviceAddr at a later stage
func NewClient(gatewayAddr net.IP) (client *Client, err error) {
	udpAddr := &net.UDPAddr{IP: gatewayAddr, Port: 5351}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
	eventChan := make(chan Event)

	mappings := make(map[uint16]PortMap)

	clientEpoch := &ClientEpoch{}

	nonce, err := genRandomBytes(12)
	if err != nil {
		return nil, ErrNonceGeneration
	}

	client = &Client{gatewayAddr, eventChan, mappings, conn, false, clientEpoch, nonce}

	go client.handleMessage()
	//Need to create mapping refresh loop. Should only refresh mappings with active = true by default.
	go client.checkMappings()
	return client, nil
}

func (c *Client) checkMappings() (err error) {
	for {
		for k, v := range c.Mappings {
			//If mapping is active and due to expire within 10 minutes, refresh
			//NB: This is not spec compliant - read 11.2.1 of RFC6887 for timing spec.
			//Will be implemented at a later date
			//Probably easiest to add a nextRefresh parameter to PortMap to accomodate timing.
			t := time.Now()
			if v.active && v.expireTime <= t.Unix() + 600 {
				log.Debugf("Refreshing mapping for port: %d", k)
				//c.RefreshMapping()
			}
		}
		//Run once a second
		time.Sleep(time.Second)
	}
}

func (c *Client) handleMessage() (err error) {
	ch := make(chan []byte)
	go func() {
		for {
			if c.cancelled {
				close(ch)
				break
			}
			select {
			case <-time.After(10 * time.Millisecond):
				msg := make([]byte, 2048)
				len, from, err := c.conn.ReadFromUDP(msg)
				if err != nil {
					log.Debugf("Error occurred when receiving UDP packet: %s\n", err)
					continue
				}
				//Seems to be the only thing that works. Should fix proerly in future.
				if fmt.Sprintf("%x", from.IP) != fmt.Sprintf("%x", c.GatewayAddr) {
					log.Debug(ErrAddressMismatch)
					continue
				}
				msg = msg[:len]
				ch <- msg
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	for {
		select {
		case msg := <-ch:
			var res ResponsePacket
			err = res.unmarshal(msg)
			if err != nil {
				log.Debug(ErrWrongPacketType)
				continue
			}
			//Process ResponsePacket here and send events.
			switch res.opCode {
			case OpAnnounce:
				//OpAnnounce case
			case OpMap:
				//OpMap case
				var data OpDataMap
				err = data.unmarshal(res.opData)
				if err != nil {
					log.Errorf("Could not parse Map OpData: %s\n", err)
					continue
				}
				if m, ok := c.Mappings[data.internalPort]; ok {
					m.externalPort = data.externalPort
					m.externalIP = data.externalIP
					m.active = true
					m.lifetime = res.lifetime
					t := time.Now()
					m.expireTime = t.Unix() + int64(res.lifetime)

					c.Mappings[data.internalPort] = m
					c.Event <- Event{ActionReceivedMapping, m}
				} else {
					log.Warnf("Port mapping was not found in client cache. Ignoring. Port: %d", data.internalPort)
				}
			case OpPeer:
				//OpPeer case
			default:
				log.Warnf("Unrecognised OpCode: %d", res.opCode)
			}
			t := time.Now()
			valid := c.epochValid(t.Unix(), res.epoch)
			if !valid {
				//PCP server lost state. Refresh mappings.
				log.Debug("Invalid epoch received. Refreshing mappings.")
			} else {
				log.Debugf("Epoch valid. Server Time: %d, Client Time: %d", res.epoch, t.Unix())
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *Client) sendMessage(msg []byte) (err error) {
	_, err = c.conn.Write(msg)
	return
}

//Closes connection to PCP server and closes event channel
func (c *Client) Close() {
	c.conn.Close()
	close(c.Event)
	c.cancelled = true
}

func genRandomBytes(size int) (blk []byte, err error) {
	blk = make([]byte, size)
	_, err = rand.Read(blk)
	return
}
