package main

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

//Potentially add deviceAddr at a later stage
func NewClient() (client *Client, err error) {
	gatewayAddr, err := client.GetGatewayAddress()
	if err != nil {
		return nil, err
	}
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
	go client.checkMappings()
	return client, nil
}

func (c *Client) checkMappings() (err error) {
	for {
		for k, v := range c.Mappings {
			t := time.Now()
			if v.active && v.refresh.time <= t.Unix() {
				log.Debugf("Refreshing mapping for port: %d", k)
				err = c.RefreshPortMapping(v.internalPort, v.lifetime)
				if err != nil {
					log.Errorf("Error occured whilst refreshing mapping: %s", err)
				}
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
			time.Sleep(time.Millisecond)
		}
	}()
	for {
		select {
		case msg := <-ch:
			var res ResponsePacket
			err = res.unmarshal(msg)
			if err != nil {
				if err == ErrUnsupportedVersion {
					log.Fatal("Server uses an unsupported PCP version.")
					os.Exit(1)
				} else {
					log.Error(err)
				}
				continue
			}
			//Need to add check for resultcode here and handle errors.
			switch res.resultCode {
			case ResultSuccess:
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

					rt := RefreshTime{0,getRefreshTime(0, res.lifetime)}
					m := PortMap{data.protocol,data.internalPort,data.externalPort,data.externalIP,true,res.lifetime,rt}
					c.Mappings[data.internalPort] = m
					c.Event <- Event{ActionReceivedMapping, m}
				case OpPeer:
					//OpPeer case
				default:
					log.Warnf("Unrecognised OpCode: %d", res.opCode)
				}
			case ResultUnsupportedVersion:
					log.Fatal("Server uses an unsupported PCP version.")
					os.Exit(1)
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
