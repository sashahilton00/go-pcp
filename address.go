package main

import(
  "net"

  "github.com/jackpal/gateway"
)

func GetGatewayAddress() (addr net.IP, err error) {
  addr, err = gateway.DiscoverGateway()
  if err != nil {
    return nil, ErrGatewayNotFound
  }
  return addr, nil
}

func GetExternalAddress() {
  // Placeholder:
  // Will create a short mapping with PCP server and return the address returned
  // by the server in the response packet. Use UDP/9 (Discard) as short mapping.
}

func GetInternalAddress() (addr net.IP, err error) {
  gatewayAddr, err := GetGatewayAddress()
  if err != nil {
    return nil, err
  }

  ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			switch x := addr.(type) {
			case *net.IPNet:
				if x.Contains(gatewayAddr) {
					return x.IP, nil
				}
			}
		}
	}

	return nil, ErrNoInternalAddress
}
