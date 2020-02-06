package main

import(
  "log"
  "net"
  "time"
)

const(
  DefaultLifetimeSeconds = 3600
)

func main() {
  addr, err := GetInternalAddress()
  if err != nil {
    log.Fatal(err)
  }
  gatewayAddr, err := GetGatewayAddress()
  if err != nil {
    log.Fatal(err)
  }
  log.Printf("Internal IP: %s Gateway IP: %s", addr, gatewayAddr)
  //log.Println(OpMap, int(OpMap))

  rp := &RequestPacket{OpCode(OpAnnounce),DefaultLifetimeSeconds,addr,[]byte{0xaa,0xbb,0xcc,0xdd},nil}
  msg, err := rp.marshal()
  if err != nil {
    log.Fatal(err)
  }
  log.Printf("%x\n", msg)
  client, err := NewClient(net.ParseIP("127.0.0.1"))
  if err != nil {
    log.Println(err)
  }
  _ = client.sendMessage(msg)
  for {
    time.Sleep(time.Millisecond)
  }
  //Need to create response packet
  /*var res ResponsePacket
  err = res.unmarshal(msg)
  if err != nil {
    log.Fatal(err)
  }
  log.Println(res)*/
}
