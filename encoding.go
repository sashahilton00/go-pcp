package pcp

import (
	"encoding/binary"
	"net"

	"github.com/boljen/go-bitmap"
	log "github.com/sirupsen/logrus"
)

const (
	ProtocolAll Protocol = 0
	ProtocolTCP Protocol = 6
	ProtocolUDP Protocol = 17
)

type OpCode uint8
type OptionOpCode uint8

//At a later date it would be worth integrating with the IANA package to be fully compliant.
//For now, just implement common protocols
type Protocol uint8
type ResultCode uint8

type OpDataMap struct {
	Protocol     Protocol
	InternalPort uint16
	ExternalPort uint16 //This is only a suggestion in request. Server ultimately decides.
	ExternalIP   net.IP //Also only a suggestion
}

type OpDataPeer struct {
	OpDataMap
	RemotePort uint16
	RemoteIP   net.IP
}

//Potentially add in progress bool
type RefreshTime struct {
	Attempt int
	Time    int64
}

type PortMap struct {
	OpDataMap
	Active   bool
	Lifetime uint32
	Refresh  RefreshTime
}

type PeerMap struct {
	PortMap
	RemotePort uint16
	RemoteIP   net.IP
}

func (o OpCode) String() string {
	return [...]string{"OpAnnounce", "OpMap", "OpPeer"}[o]
}

func (o OptionOpCode) String() string {
	return [...]string{"OptionOpReserved", "OptionOpThirdParty", "OptionOpPreferFailure", "OptionOpFilter", "OptionOpNonce", "OptionOpAuthenticationTag", "OptionOpPaAuthenticationTag", "OptionOpEapPayload", "OptionOpPrf", "OptionOpMacAlgorithm", "OptionOpSessionLifetime", "OptionOpReceivedPak", "OptionOpIdIndicator", "OptionOpThirdPartyId"}[o]
}

func (p Protocol) String() string {
	return [...]string{"ProtocolAll", "ProtocolTCP", "ProtocolUDP"}[p]
}

func (r ResultCode) String() string {
	return [...]string{"ResultSuccess", "ResultUnsupportedVersion", "ResultNotAuthorised", "ResultMalformedRequest", "ResultUnsupportedOpcode", "ResultUnsupportedOption", "ResultMalformedOption", "ResultNetworkFailure", "ResultNoResources", "ResultUnsupportedProtocol", "ResultUserExceededQuota", "ResultCannotProvideExternal", "ResultAddressMismatch", "ResultExcessiveRemotePeers"}[r]
}

const (
	DefaultLifetimeSeconds = 3600
)

const (
	OpAnnounce OpCode = iota
	OpMap
	OpPeer
)

const (
	OptionOpReserved OptionOpCode = iota
	OptionOpThirdParty
	OptionOpPreferFailure
	OptionOpFilter
	OptionOpNonce
	OptionOpAuthenticationTag
	OptionOpPaAuthenticationTag
	OptionOpEapPayload
	OptionOpPrf
	OptionOpMacAlgorithm
	OptionOpSessionLifetime
	OptionOpReceivedPak
	OptionOpIdIndicator
	OptionOpThirdPartyId
	//Currently not implementing 128+
)

const (
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
	opCode OptionOpCode
	data   []byte
}

type RequestPacket struct {
	opCode     OpCode
	lifetime   uint32
	clientAddr net.IP
	opData     []byte
	pcpOptions []PCPOption
}

type ResponsePacket struct {
	opCode     OpCode
	resultCode ResultCode
	lifetime   uint32
	epoch      uint32
	opData     []byte
	pcpOptions []PCPOption
}

func (data *OpDataMap) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	//Also, marshal is probably the wrong place for this, since it technically doesn't cause an error.
	if data.InternalPort == 0 {
		return nil, ErrPortNotSpecified
	}

	empty := make([]byte, 3)

	internalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(internalPortBytes, data.InternalPort)

	externalPortBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(externalPortBytes, data.ExternalPort)

	addr := make([]byte, 16)
	if data.ExternalIP != nil {
		copy(addr, data.ExternalIP)
	}

	var slices = [][]byte{
		nonce,
		[]byte{byte(data.Protocol)},
		empty,
		internalPortBytes,
		externalPortBytes,
		addr,
	}

	msg = concatCopyPreAllocate(slices)
	return
}

func (data *OpDataMap) unmarshal(msg []byte) (err error) {
	data = &OpDataMap{
		Protocol:     Protocol(msg[12]),
		InternalPort: binary.BigEndian.Uint16(msg[16:18]),
		ExternalPort: binary.BigEndian.Uint16(msg[18:20]),
		ExternalIP:   net.IP(msg[20:36]),
	}
	return
}

func (data *OpDataPeer) marshal(nonce []byte) (msg []byte, err error) {
	//Potentially relax requirement for non-zero. Appears to be valid in the spec when combined with ProtocolAll.
	//Also, marshal is probably the wrong place for this, since it technically doesn't cause an error.
	if data.InternalPort == 0 {
		return nil, ErrPortNotSpecified
	}

	r1, r2 := make([]byte, 3), make([]byte, 2)

	internalPortBytes, externalPortBytes, remotePortBytes := make([]byte, 2), make([]byte, 2), make([]byte, 2)

	binary.BigEndian.PutUint16(internalPortBytes, data.InternalPort)
	binary.BigEndian.PutUint16(externalPortBytes, data.ExternalPort)
	binary.BigEndian.PutUint16(remotePortBytes, data.RemotePort)

	addr, remoteAddr := make([]byte, 16), make([]byte, 16)
	if data.ExternalIP != nil {
		copy(addr, data.ExternalIP)
	}
	if data.RemoteIP == nil {
		return nil, ErrNoAddress
	}
	copy(remoteAddr, data.RemoteIP)

	var slices = [][]byte{
		nonce,
		[]byte{byte(data.Protocol)},
		r1,
		internalPortBytes,
		externalPortBytes,
		addr,
		remotePortBytes,
		r2,
		remoteAddr,
	}

	msg = concatCopyPreAllocate(slices)
	return
}

func (data *OpDataPeer) unmarshal(msg []byte) (err error) {
	data = &OpDataPeer{
		OpDataMap: OpDataMap{
			Protocol:     Protocol(msg[12]),
			InternalPort: binary.BigEndian.Uint16(msg[16:18]),
			ExternalPort: binary.BigEndian.Uint16(msg[18:20]),
			ExternalIP:   net.IP(msg[20:36]),
		},
		RemotePort: binary.BigEndian.Uint16(msg[36:38]),
		RemoteIP:   net.IP(msg[40:56]),
	}
	return
}

func (req *RequestPacket) marshal() (msg []byte, err error) {
	opMap := bitmap.NewSlice(8)
	//Bits at indexes 0-6 set from opCode int.
	for i := 0; i < 7; i++ {
		opCodeBit := bitmap.GetBit(byte(req.opCode), i)
		bitmap.Set(opMap, i, opCodeBit)
	}
	//Bit at index 7 is 0 as it is a request
	bitmap.Set(opMap, 7, false)

	empty := make([]byte, 2)

	lifetimeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lifetimeBytes, req.lifetime)

	addr := make([]byte, 16)
	log.Debugf("Client addr: %s\n", req.clientAddr)
	copy(addr, req.clientAddr)

	var options []byte
	log.Debugf("Number of options in request: %d", len(req.pcpOptions))
	for _, option := range req.pcpOptions {
		//8 bits reserved
		empty := make([]byte, 1)

		optionSlices := [][]byte{
			[]byte{byte(option.opCode)},
			empty,
			//length of option data payload
			[]byte{byte(len(option.data))},
			option.data,
		}

		optionBytes := concatCopyPreAllocate(optionSlices)
		//Pad option data to multiple of 4
		optionBytes = addPadding(optionBytes)

		options = append(options, optionBytes...)
	}

	var slices = [][]byte{
		//The current PCP version number
		[]byte{2},
		opMap,
		//Next 2 bytes (16 bits) reserved
		empty,
		//lifetime is an unsigned 32 bit integer in seconds
		lifetimeBytes,
		//client ip is always a 16 byte (128 bit) block
		addr,
		//opData is the opcode-specific data
		req.opData,
		options,
	}

	msg = concatCopyPreAllocate(slices)
	//Pad message to multiple of 4
	msg = addPadding(msg)

	if len(msg) > 1100 {
		return nil, ErrPacketTooLarge
	}
	log.Debugf("Request Bytes: %x\n", msg)
	return msg, nil
}

func (res *ResponsePacket) unmarshal(data []byte) (err error) {
	log.Debugf("Response Bytes: %x\n", data)
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
		opCode = bitmap.SetBit(opCode, i, opCodeBit)
	}
	res.opCode = OpCode(opCode)
	res.resultCode = ResultCode(data[3])
	log.Debugf("Result Code: %s", res.resultCode)
	res.lifetime = binary.BigEndian.Uint32(data[4:8])
	log.Debugf("Response Lifetime: %d", res.lifetime)
	res.epoch = binary.BigEndian.Uint32(data[8:12])
	log.Debugf("Response Epoch: %d", res.epoch)

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

	log.Debugf("Opcode: %s\n", res.opCode)
	log.Debugf("Op data len: %d\n", opDataLen)
	if opDataLen > 0 {
		res.opData = data[24 : 24+opDataLen]
	}

	currentOffset := 24 + opDataLen
	for currentOffset < len(data) {
		log.Debugf("Current offset: %d\n", currentOffset)
		log.Debugf("Remaining data: %x\n", data[currentOffset:])
		opCode := OptionOpCode(data[currentOffset])
		log.Debugf("Option OpCode: %s\n", opCode)
		optionLengthBytes := make([]byte, 2)
		copy(optionLengthBytes, data[currentOffset+2:currentOffset+3])
		optionLength := binary.BigEndian.Uint16(optionLengthBytes)
		log.Debugf("Option data length: %d", optionLength)
		var optionData []byte
		dataStart := currentOffset + 3
		if optionLength > 0 {
			dataEnd := dataStart + int(optionLength)
			optionData = data[dataStart:dataEnd]
			currentOffset = dataEnd
		} else {
			currentOffset = dataStart
		}
		//OpCode 0 is reserved, has no function, hence dropped due to possible
		//entries from reading empty bytes
		if opCode != 0 {
			option := PCPOption{opCode, optionData}
			res.pcpOptions = append(res.pcpOptions, option)
		}
	}

	return
}
