package main

import(
  "crypto/rand"
  "encoding/binary"
  "net"

  log "github.com/sirupsen/logrus"
)

//At a later date it would be worth integrating with the IANA package to be fully compliant.
//For now, just implement common protocols
type Protocol uint8

func (p Protocol) String() string {
    return [...]string{"ProtocolAll","ProtocolTCP","ProtocolUDP"}[p]
}

const(
  ProtocolAll Protocol = 0
  ProtocolTCP Protocol = 6
  ProtocolUDP Protocol = 17
)

type OpDataMap struct {
  nonce []byte
  protocol Protocol
  internalPort uint16
  externalPort uint16 //This is only a suggestion in request. Server ultimately decides.
  externalIP net.IP //Also only a suggestion
}

func (data *OpDataMap) marshal() (msg []byte, err error) {
  //Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
  if data.internalPort == 0 {
    return nil, ErrPortNotSpecified
  }
  if data.nonce == nil {
    nonce, err := genRandomBytes(12)
    if err != nil {
      return nil, ErrNonceGeneration
    }
    data.nonce = nonce
  }
  msg = append(msg, data.nonce...)
  msg = append(msg, byte(data.protocol))
  empty := make([]byte, 3)
  msg = append(msg, empty...)
  internalPortBytes := make([]byte, 2)
  binary.BigEndian.PutUint16(internalPortBytes, data.internalPort)
  msg = append(msg, internalPortBytes...)
  externalPortBytes := make([]byte, 2)
  binary.BigEndian.PutUint16(externalPortBytes, data.externalPort)
  msg = append(msg, externalPortBytes...)
  addr := make([]byte, 16)
  if data.externalIP != nil {
    copy(addr, data.externalIP)
  }
  msg = append(msg, addr...)
  return
}

//Need to add support for PCP options later.
func (c *Client) AddPortMapping(protocol Protocol, internalPort, requestedExternalPort uint16, requestedAddr net.IP, lifetime uint32) (err error) {
  mapData := &OpDataMap{nil,protocol,internalPort,requestedExternalPort,requestedAddr}
  mapDataBytes, err := mapData.marshal()
  if err != nil {
    return ErrMapDataPayload
  }
  addr, err := GetInternalAddress()
  if err != nil {
    return ErrNoInternalAddress
  }
  requestData := &RequestPacket{OpCode(OpMap),lifetime,addr,mapDataBytes,nil}
  requestDataBytes, err := requestData.marshal()
  if err != nil {
    return ErrRequestDataPayload
  }
  log.Debugf("Request bytes: %x Opcode: %s", requestDataBytes, OpCode(OpMap))
  err = c.sendMessage(requestDataBytes)
  if err != nil {
    return ErrNetworkSend
  }
  return
}

func genRandomBytes(size int) (blk []byte, err error) {
    blk = make([]byte, size)
    _, err = rand.Read(blk)
    return
}
