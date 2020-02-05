package main

import(
  "log"
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

  rp := &RequestPacket{OpCode(OpMap),DefaultLifetimeSeconds,addr,[]byte{0xaa,0xbb,0xcc,0xdd},nil}
  msg, err := rp.marshal()
  if err != nil {
    log.Fatal(err)
  }
  log.Printf("%x\n", msg)
  //Need to create response packet
  /*var res ResponsePacket
  err = res.unmarshal(msg)
  if err != nil {
    log.Fatal(err)
  }
  log.Println(res)*/
}
