package main

import (
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
	//client, err := NewClient(gatewayAddr)
	if err != nil {
		log.Error(err)
	}
	err = client.AddPortMapping(ProtocolTCP, 8080, 0, net.ParseIP("127.0.0.1"), DefaultLifetimeSeconds)
	if err == nil {
		log.Debug("successfully sent port map request")
	}
	log.Debugf("%+v\n", client)
	for {
		select {
		case event := <-client.Event:
			log.Infof("Received event - Action: %s, Data: %+v", event.Action, event.Data)
		}
		time.Sleep(time.Millisecond)
	}
}
