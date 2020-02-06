package main

import(
  "net"
  "time"

  log "github.com/sirupsen/logrus"
)

func init() {
  log.SetLevel(log.DebugLevel)
}

func main() {
  addr, err := GetInternalAddress()
  if err != nil {
    log.Fatal(err)
  }
  gatewayAddr, err := GetGatewayAddress()
  if err != nil {
    log.Fatal(err)
  }
  log.Infof("Internal IP: %s Gateway IP: %s", addr, gatewayAddr)
  //Only temporary, for testing with local server
  client, err := NewClient(net.ParseIP("127.0.0.1"))
  if err != nil {
    log.Error(err)
  }
  err = client.AddPortMapping(ProtocolTCP, 8080, 0, net.ParseIP("127.0.0.2"), DefaultLifetimeSeconds)
  if err == nil {
    log.Debug("successfully sent port map request")
  }
  //_ = client.sendMessage(msg)
  for {
    time.Sleep(time.Millisecond)
  }
}
