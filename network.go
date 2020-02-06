package main

import(
  "fmt"
  "log"
  "net"
  "time"
)

type Event struct {
  Action string
  Data []byte
}

type Client struct {
  GatewayAddr net.IP
  Event chan Event

  conn *net.UDPConn
  cancelled bool
}

//Potentially add deviceAddr at a later stage
func NewClient(gatewayAddr net.IP) (client *Client, err error) {
  udpAddr := &net.UDPAddr{IP: gatewayAddr, Port: 5351}
  conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}
  eventChan := make(chan Event)
  client = &Client{gatewayAddr,eventChan,conn,false}
  //Need to set up event handler loop here for incoming messages.
  go client.readMessage()
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
          log.Printf("Error occurred when receiving UDP packet: %s\n", err)
    			continue
    		}
        //Seems to be the only thing that works. Should fix proerly in future.
        if fmt.Sprintf("%x", from.IP) != fmt.Sprintf("%x", c.GatewayAddr) {
          log.Println(ErrAddressMismatch)
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
        log.Println(ErrWrongPacketType)
        continue
      }
      //Process ResponsePacket here and send events.
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
