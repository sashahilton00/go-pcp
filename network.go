package main

import(
  "fmt"
  "net"
  "time"

  log "github.com/sirupsen/logrus"
)

type Event struct {
  Action string
  Data []byte
}

type ClientEpoch struct {
  prevServerTime uint32
  prevClientTime int64
}

type Client struct {
  GatewayAddr net.IP
  Event chan Event

  conn *net.UDPConn
  cancelled bool
  epoch *ClientEpoch
}

//Potentially add deviceAddr at a later stage
func NewClient(gatewayAddr net.IP) (client *Client, err error) {
  udpAddr := &net.UDPAddr{IP: gatewayAddr, Port: 5351}
  conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
  eventChan := make(chan Event)
  clientEpoch := &ClientEpoch{}
  client = &Client{gatewayAddr,eventChan,conn,false,clientEpoch}
  //Need to set up event handler loop here for incoming messages.
  go client.readMessage()
  //Need to create mapping refresh loop
  return client, nil
}

//Should rename to handleMessage
func (c *Client) readMessage() (err error) {
  //Listens for messages from UDP conn.
  //Creates Event depending on message.
  //Emit event to channel.
  ch := make(chan []byte)
  // Read incoming UDP messages
  go func() {
    for {
      if c.cancelled {
        close(ch)
        break
      }
    	select {
    	case <-time.After(10 * time.Millisecond):
    		// do something
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
