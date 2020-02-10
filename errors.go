package main

import (
	"errors"
)

var (
	ErrNoInternalAddress  = errors.New("no internal address")
	ErrNoExternalAddress  = errors.New("no external address")
	ErrGatewayNotFound    = errors.New("gateway not found")
	ErrPacketTooLarge     = errors.New("packet exceeds 1100 octet size limit")
	ErrUnsupportedVersion = errors.New("the specified version is not supported")
	ErrWrongPacketType    = errors.New("the packet is not of the correct type")
	ErrAddressMismatch    = errors.New("the sender and gateway addresses do not match")
	ErrPortNotSpecified   = errors.New("you must specify a port")
	ErrNonceGeneration    = errors.New("an error occured whilst generating nonce")
	ErrMapDataPayload     = errors.New("could not create the map data payload")
	ErrRequestDataPayload = errors.New("could not create the request data payload")
	ErrNetworkSend        = errors.New("an error occured whilst sending the network request")
	ErrMappingNotFound    = errors.New("mapping not found")
)
