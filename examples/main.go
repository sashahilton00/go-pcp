package main

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	. "github.com/sashahilton00/go-pcp"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	var client *Client
	client, err := NewClient()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	addr, err := client.GetInternalAddress()
	if err != nil {
		log.Fatal(err)
	}
	gatewayAddr, err := client.GetGatewayAddress()
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Internal IP: %s Gateway IP: %s", addr, gatewayAddr)

	err = client.AddPortMapping(ProtocolTCP, 8080, 0, nil, DefaultLifetimeSeconds)
	if err == nil {
		log.Debug("successfully sent port map request")
	}

	addr, err = client.GetExternalAddress()
	if err == nil {
		log.Infof("External Addr: %s", addr)
	} else {
		log.Errorf("err retrieving address: %s", err)
	}

	for {
		select {
		case event := <-client.Event:
			log.Infof("Received event - Action: %s, Data: %+v", event.Action, event.Data)
		}
		time.Sleep(time.Millisecond)
	}
}
