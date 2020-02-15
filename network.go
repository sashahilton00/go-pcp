package pcp

import (
	"fmt"
	"net"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

//Potentially add deviceAddr at a later stage
func NewClient(timeout int) (client *Client, err error) {
	gatewayAddr, err := client.GetGatewayAddress()
	if err != nil {
		return nil, err
	}
	udpAddr := &net.UDPAddr{IP: gatewayAddr, Port: 5351}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
	cancelChan, eventChan := make(chan bool), make(chan Event)

	mappings := make(map[uint16]PortMap)
	peerMappings := make(map[uint16]PeerMap)

	clientEpoch := &ClientEpoch{}

	nonce, err := genRandomBytes(12)
	if err != nil {
		return nil, ErrNonceGeneration
	}

	client = &Client{gatewayAddr, eventChan, mappings, peerMappings, conn, cancelChan, clientEpoch, nonce}

	client.Announce()
	for {
		select {
		case event := <-client.Event:
			if event.Action == ActionReceivedAnnounce {
				go client.handleMessage()
				go client.checkMappings()
				return client, nil
			}
		case <- time.After(time.Duration(timeout) * time.Second):
			return nil, ErrNetworkTimeout
		}
		time.Sleep(time.Millisecond)
	}

}

func (c *Client) checkMappings() (err error) {
	for {
		for k, v := range c.Mappings {
			t := time.Now()
			if v.Active && v.Refresh.Time <= t.Unix() {
				log.Debugf("Refreshing mapping for port: %d", k)
				err = c.RefreshPortMapping(v.InternalPort, v.Lifetime)
				if err != nil {
					log.Errorf("Error occured whilst refreshing mapping: %s", err)
				}
			}
		}
		time.Sleep(time.Second)
	}
}

func (c *Client) handleMessage() (err error) {
	ch := make(chan []byte)
	go func() {
		for {
			select {
			case <- c.cancelled:
				close(ch)
				break
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
					log.Debug("Announce Opcode received.")
					c.Event <- Event{ActionReceivedAnnounce, nil}
				case OpMap:
					var data OpDataMap
					err = data.unmarshal(res.opData)
					if err != nil {
						log.Errorf("Could not parse Map OpData: %s\n", err)
						continue
					}

					rt := RefreshTime{
						Attempt: 0,
						Time:    getRefreshTime(0, res.lifetime),
					}
					m := PortMap{
						OpDataMap: OpDataMap{
							Protocol:     data.Protocol,
							InternalPort: data.InternalPort,
							ExternalPort: data.ExternalPort,
							ExternalIP:   data.ExternalIP,
						},
						Active:   true,
						Lifetime: res.lifetime,
						Refresh:  rt,
					}
					c.Mappings[data.InternalPort] = m
					c.Event <- Event{ActionReceivedMapping, m}
				case OpPeer:
					var data OpDataPeer
					err = data.unmarshal(res.opData)
					if err != nil {
						log.Errorf("Could not parse Peer OpData: %s\n", err)
						continue
					}

					rt := RefreshTime{
						Attempt: 0,
						Time:    getRefreshTime(0, res.lifetime),
					}
					m := PeerMap{
						PortMap: PortMap{
							OpDataMap: OpDataMap{
								Protocol:     data.Protocol,
								InternalPort: data.InternalPort,
								ExternalPort: data.ExternalPort,
								ExternalIP:   data.ExternalIP,
							},
							Active:   true,
							Lifetime: res.lifetime,
							Refresh:  rt,
						},
						RemotePort: data.RemotePort,
						RemoteIP: data.RemoteIP,
					}
					c.PeerMappings[data.InternalPort] = m
					c.Event <- Event{ActionReceivedPeer, m}
				default:
					log.Warnf("Unrecognised OpCode: %d", res.opCode)
				}
			case ResultUnsupportedVersion:
				log.Fatal("Server uses an unsupported PCP version.")
				os.Exit(1)
			default:
				log.Debugf("Non success ResultCode received. ResultCode %s", res.resultCode)
			}

			t := time.Now()
			valid := c.epochValid(t.Unix(), res.epoch)
			if !valid {
				log.Debug("Invalid epoch received. Refreshing mappings.")
				c.refreshMappings()
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

//Closes connection to PCP server and returns close event
func (c *Client) Close() {
	c.cancelled <- true
	c.Event <- Event{
		Action: ActionClose,
		Data: nil,
	}
	return
}
