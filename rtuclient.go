// Copyright 2014 Quoc-Viet Nguyen. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD license.  See the LICENSE file for details.
package modbus

import (
	"fmt"
	"log"
	"time"
)

const (
	rtuMinLength = 4
	rtuMaxLength = 256

	rtuTimeoutMillis = 5000
)

type RTUClientHandler struct {
	rtuPackager
	// TODO: sharing serial transporter with Modbus ASCII client
	rtuSerialTransporter
}

func RTUClient(address string) Client {
	handler := &RTUClientHandler{}
	handler.Address = address
	return RTUClientWithHandler(handler)
}

func RTUClientWithHandler(handler *RTUClientHandler) Client {
	return NewClient(handler, handler)
}

// Implements Encoder and Decoder interface
type rtuPackager struct {
	SlaveId byte
}

// Encode encodes PDU in a RTU frame:
//  Address         : 1 byte
//  Function        : 1 byte
//  Data            : 0 up to 252 bytes
//  CRC             : 2 byte
func (mb *rtuPackager) Encode(pdu *ProtocolDataUnit) (adu []byte, err error) {
	length := len(pdu.Data) + 4
	if length > rtuMaxLength {
		err = fmt.Errorf("modbus: length of data '%v' must not be bigger than '%v'", length, rtuMaxLength)
		return
	}
	adu = make([]byte, length)

	adu[0] = mb.SlaveId
	adu[1] = pdu.FunctionCode
	copy(adu[2:], pdu.Data)

	// Append crc
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := crc.value()

	adu[length-2] = byte(checksum >> 8)
	adu[length-1] = byte(checksum)
	return
}

// Verify verifies response length and slave id
func (mb *rtuPackager) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	length := len(aduResponse)
	// Minimum size (including address, function and CRC)
	if length < rtuMinLength {
		err = fmt.Errorf("modbus: response length '%v' does not meet minimum '%v'", length, rtuMinLength)
		return
	}
	// Slave address must match
	if aduResponse[0] != aduRequest[0] {
		err = fmt.Errorf("modbus: response slave id '%v' does not match request '%v'", aduResponse[0], aduRequest[0])
		return
	}
	return
}

// Decode extracts PDU from RTU frame and verify CRC
func (mb *rtuPackager) Decode(adu []byte) (pdu *ProtocolDataUnit, err error) {
	length := len(adu)
	// Calculate checksum
	var crc crc
	crc.reset().pushBytes(adu[0 : length-2])
	checksum := uint16(adu[length-2])<<8 | uint16(adu[length-1])
	if checksum != crc.value() {
		err = fmt.Errorf("modbus: response crc '%v' does not match expected '%v'", checksum, crc.value())
		return
	}
	// Function code & data
	pdu = &ProtocolDataUnit{}
	pdu.FunctionCode = adu[1]
	pdu.Data = adu[2 : length-2]
	return
}

// asciiSerialTransporter implements Transporter interface
type rtuSerialTransporter struct {
	// Serial port configuration
	serialConfig
	// Read timeout
	Timeout time.Duration
	Logger  *log.Logger

	// Serial controller
	serial serial
}

func (mb *rtuSerialTransporter) Send(aduRequest []byte) (aduResponse []byte, err error) {
	if mb.serial.IsConnected() {
		// flush current data pending in serial port
	} else {
		if err = mb.Connect(); err != nil {
			return
		}
	}
	if mb.Logger != nil {
		mb.Logger.Printf("modbus: sending %v\n", aduRequest)
	}
	var n int
	if n, err = mb.serial.Write(aduRequest); err != nil {
		return
	}
	var data [rtuMaxLength]byte
	if n, err = mb.serial.Read(data[:]); err != nil {
		return
	}
	aduResponse = data[:n]
	if mb.Logger != nil {
		mb.Logger.Printf("modbus: received %v\n", aduResponse)
	}
	return
}

func (mb *rtuSerialTransporter) Connect() (err error) {
	if mb.Logger != nil {
		mb.Logger.Printf("modbus: connecting '%v'\n", mb.serialConfig.Address)
	}
	// Timeout is required
	if mb.Timeout <= 0 {
		mb.Timeout = rtuTimeoutMillis * time.Millisecond
	}
	// Transfer timeout setting to serial backend
	mb.serial.Timeout = mb.Timeout
	err = mb.serial.Connect(&mb.serialConfig)
	return
}

func (mb *rtuSerialTransporter) Close() (err error) {
	err = mb.serial.Close()
	if mb.Logger != nil {
		mb.Logger.Printf("modbus: closed connection '%v'\n", mb.serialConfig.Address)
	}
	return
}