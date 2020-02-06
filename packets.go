package main

import(
  "encoding/binary"
  "net"

  "github.com/boljen/go-bitmap"
)

type OpCode uint8
type ResultCode uint8

func (o OpCode) String() string {
    return [...]string{"Announce","Map","Peer"}[o]
}

func (r ResultCode) String() string {
    return [...]string{"ResultSuccess","ResultUnsupportedVersion","ResultNotAuthorised","ResultMalformedRequest","ResultUnsupportedOpcode","ResultUnsupportedOption","ResultMalformedOption","ResultNetworkFailure","ResultNoResources","ResultUnsupportedProtocol","ResultUserExceededQuota","ResultCannotProvideExternal","ResultAddressMismatch","ResultExcessiveRemotePeers"}[r]
}

const(
  OpAnnounce OpCode = iota
  OpMap
  OpPeer
)

const(
  ResultSuccess ResultCode = iota
  ResultUnsupportedVersion
  ResultNotAuthorised
  ResultMalformedRequest
  ResultUnsupportedOpcode
  ResultUnsupportedOption
  ResultMalformedOption
  ResultNetworkFailure
  ResultNoResources
  ResultUnsupportedProtocol
  ResultUserExceededQuota
  ResultCannotProvideExternal
  ResultAddressMismatch
  ResultExcessiveRemotePeers
)


type PCPOption struct {
  opCode OpCode
  data []byte
}

type RequestPacket struct {
  //version int8 //Probably not necessary - just add when marshalling.
  opCode OpCode //Should convert this to take an enum.
  lifetime uint32
  clientAddr net.IP
  opData []byte
  pcpOptions []PCPOption
}

type ResponsePacket struct {
  opCode OpCode
  resultCode ResultCode
  lifetime uint32
  epoch uint32
  opData []byte
  pcpOptions []PCPOption
}

//Necessary for padding all messages to multiple of 4 octets
func addPadding(data []byte) (out []byte) {
  length := len(data)
  padding := 4 - (length % 4)
  if padding > 0 {
    empty := make([]byte, padding)
    out = append(data, empty...)
  }
  return out
}

//The logic in here is a mess, need to redo to deal with endianness
func (req *RequestPacket) marshal() (msg []byte, err error) {
  //The current PCP version number
  msg = append(msg, 2)
  opMap := bitmap.NewSlice(8)
  //Bits at indexes 0-6 set from opCode int.
  for i := 0; i < 7; i++ {
    opCodeBit := bitmap.GetBit(byte(req.opCode), i)
    bitmap.Set(opMap, i, opCodeBit)
  }
  //Bit at index 7 is 0 as it is a request
  bitmap.Set(opMap, 7, false)
  msg = append(msg, opMap...)
  //Next 2 bytes (16 bits) reserved
  empty := make([]byte, 2)
  msg = append(msg, empty...)
  //lifetime is an unsigned 32 bit integer in seconds
  lifetimeBytes := make([]byte, 4)
  binary.BigEndian.PutUint32(lifetimeBytes, req.lifetime)
  msg = append(msg, lifetimeBytes...)
  //client ip is always a 16 byte (128 bit) block
  addr := make([]byte, 16)
  copy(addr, req.clientAddr)
  msg = append(msg, addr...)
  //opData is the opcode-specific data
  msg = append(msg, req.opData...)
  //Encode and append the options
  var options []byte
  for _, option := range req.pcpOptions {
    var optionBytes []byte
    optionBytes = append(optionBytes, byte(option.opCode))
    //8 bits reserved
    empty := make([]byte, 1)
    optionBytes = append(optionBytes, empty...)
    //length of option data payload
    optionBytes = append(optionBytes, uint8(len(option.data)))
    optionBytes = append(optionBytes, option.data...)
    //Pad option data to multiple of 4
    optionBytes = addPadding(optionBytes)
    options = append(options, optionBytes...)
  }
  msg = append(msg, options...)
  //Pad message to multiple of 4
  msg = addPadding(msg)
  if len(msg) > 1100 {
    return nil, ErrPacketTooLarge
  }
  return msg, nil
}

func (res *ResponsePacket) unmarshal(data []byte) (err error) {
  //log.Printf("%x\n", data)
  version := uint8(data[0])
  if version != 2 {
    return ErrUnsupportedVersion
  }
  if !bitmap.GetBit(data[1], 7) {
    return ErrWrongPacketType
  }
  var opCode byte
  for i := 0; i < 7; i++ {
    opCodeBit := bitmap.GetBit(data[1], i)
    bitmap.SetBit(opCode, i, opCodeBit)
  }
  res.opCode = OpCode(opCode)
  res.resultCode = ResultCode(data[3])
  res.lifetime = binary.BigEndian.Uint32(data[4:8])
  res.epoch = binary.BigEndian.Uint32(data[8:12])
  var opDataLen int
  //This could be trimmed down - left for clarity.
  switch res.opCode {
  case OpAnnounce:
    opDataLen = 0
  case OpMap:
    opDataLen = 36
  case OpPeer:
    opDataLen = 56
  default:
    opDataLen = 0
  }
  if opDataLen > 0 {
    res.opData = data[24:24+opDataLen]
  }
  //Need to implement PCP options
  return
}
