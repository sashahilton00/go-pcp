# Go-PCP

This package implements the Port Control Protocol (PCP) version 2, as specified by [RFC6887](https://tools.ietf.org/html/rfc6887). PCP is intended to be the successor to NAT-PNP and UPnP.

The package may not yet be fully compliant; there are parts which are still to be implemented, and it needs testing, as I did not have a PCP server available when writing this, hence there may be bugs. I should be able to test it soon and fix any if they are present.

As it stands, progress is as follows:

- [x] Map Opcode implemented. Can Add/Delete/Refresh port mappings.
- [x] Peer Opcode implemented. Can Add/Delete/Refresh peer mappings.
- [ ] Announce Opcode not currently implemented.
- [ ] Provide proper events to Event chan of client.
- [ ] Implement PCP option support.
- [ ] Properly document methods.

### Known Bugs/Non-Compliant Features

- A number of the network traffic codes specified in the official have yet to be implemented. Currently the All, TCP & UDP codes are present. There is an IANA package that should make adding the remaining codes pretty simple.

- PCP Options are not implemented in the methods currently. 90% of the code is there, but the methods do not provide any means to pass in PCP options.

- The network receiving and processing of messages at the moment is not a great implementation, and may be buggy. I have yet to test. Possible sources may be incorrect padding of network packets. If someone wants to review the `handleMessage` method and improve it, I'd welcome changes. Same goes for the `epochValid` code, which I'm not sure is compliant.

- The way mapping deletion is handled at the moment feels like a hack - due to the method the specification provides for deleting a mapping (send a request to set mapping lifetime to zero), the does this, then listens to the events channel of the client and removes a mapping from the client map when a matching message is received. If someone can think of a cleaner way to do this, I'm all ears.

- Around line 76 of `network.go`, there is a string match for comparing the IP address of the client's gateway to that in the message received. A string match doesn't feel optimal. If someone knows of a better way, please feel free to correct. A simple byte comparison does not work due to the variable length of the arrays used to accommodate both IPv4 and IPv6 addresses as far as I can tell.
