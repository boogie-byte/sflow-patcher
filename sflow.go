// Copyright 2020 Sergey Vinogradov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
)

const (
	vxlanPort = 4789

	ipProtoUDP = 0x11

	etherType8021Q = 0x8100
	etherTypeIPv4  = 0x0800
	etherTypeIPv6  = 0x86DD
)

// sFlow struct diagrams could be found here:
// https://sflow.org/developers/diagrams/sFlowV5Datagram.pdf
// https://sflow.org/developers/diagrams/sFlowV5Sample.pdf
// https://sflow.org/developers/diagrams/sFlowV5FlowData.pdf

func processDatagram(c *copier) {
	// Do not process unsupported datagram versions
	if dv := c.copyUint32(); dv != 5 {
		panic(fmt.Sprintf("Unsupported datagram version %d", dv))
	}

	switch at := c.copyUint32(); at {
	case 1:
		// Copy IPv4 address (4 bytes), agentID (4 bytes),
		// sequenceNumber (4 bytes), agentUptime (4 bytes)
		c.copyBytes(16)
	case 2:
		// Copy IPv6 address (16 bytes), agentID (4 bytes),
		// sequenceNumber (4 bytes), agentUptime (4 bytes)
		c.copyBytes(28)
	default:
		panic(fmt.Sprintf("Unsupported agent address type %d", at))
	}

	sampleCount := c.copyUint32()
	for i := uint32(0); i < sampleCount; i++ {
		processSample(c)
	}
}

func processSample(c *copier) {
	enterpriseID, format := c.copyDataFormat()
	oldSampleLength := int(c.copyUint32())

	srcSampleStart := c.srcOffset()
	dstSampleStart := c.dstOffset()

	if enterpriseID != 0 {
		log.Debugf("Skipping unsupported sample enterpriseID %d", enterpriseID)
		c.copyBytesAt(oldSampleLength, srcSampleStart, dstSampleStart)
		return
	} else if format != 1 {
		log.Debugf("Skipping unsupported sample type %d", format)
		c.copyBytesAt(oldSampleLength, srcSampleStart, dstSampleStart)
		return
	}

	// Copy sampleSequenceNumber (4 bytes),
	// sampleDataSource (4 bytes), samplingRate (4 bytes),
	// samplePool (4 bytes), dropped (4 bytes), inputInterface (4 bytes),
	// outputInterface (4 bytes)
	c.copyBytes(28)

	recordCount := c.copyUint32()
	for i := uint32(0); i < recordCount; i++ {
		processRecord(c)
	}

	// Update the sample lenght field
	newSampleLength := uint32(c.dstOffset() - dstSampleStart)
	c.writeUint32At(newSampleLength, dstSampleStart-4)
}

func processRecord(c *copier) {
	enterpriseID, format := c.copyDataFormat()
	oldRecordLength := int(c.copyUint32())

	srcRecordStart := c.srcOffset()
	dstRecordStart := c.dstOffset()

	if enterpriseID != 0 {
		log.Debugf("Skipping unsupported record enterpriseID %d", enterpriseID)
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	} else if format != 1 {
		log.Debugf("Skipping unsupported record type %d", format)
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	}

	// Parse headerProtocol
	if hp := c.copyUint32(); hp != 1 {
		log.Debugf("Skipping unsupported frame type %d", hp)
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	}

	frameLength := c.copyUint32()

	// Copy payloadRemoved (4 bytes)
	c.copyBytes(4)

	oldHeaderLength := c.copyUint32()

	// Skip dstMAC (6 bytes), srcMAC (6 bytes)
	c.skip(12)

	var ipProto uint8
PARSE_FRAME:
	switch etherType := c.readUint16(); etherType {
	case etherType8021Q:
		c.skip(2)        // Skip VLANID
		goto PARSE_FRAME // Re-parse the frame
	case etherTypeIPv4:
		ihl := int(c.readUint8() & 0x0F) // IP header length in 32-bit words
		c.skip(8)                        // Skip several IPv4 header fields
		ipProto = c.readUint8()
		c.skip(ihl*4 - 10) // Skip all IPv4 fields left
	case etherTypeIPv6:
		c.skip(6) // Skip several IPv6 header fields
		ipProto = c.readUint8()
		c.skip(33) // Skip all IPv6 fields left
	default:
		log.Debugf("Skipping unsupported ethertype %d", etherType)
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	}

	if ipProto != ipProtoUDP {
		log.Debug("Skipping non-UDP packet")
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	}

	c.skip(2) // skip UDP src port (2 bytes)
	if dstUDPPort := c.readUint16(); dstUDPPort != vxlanPort {
		log.Debug("Skipping non-VXLAN packet")
		c.copyBytesAt(oldRecordLength, srcRecordStart, dstRecordStart)
		return
	}
	c.skip(12) // Skip UDP packet length (2 bytes), UDP checksum (2 bytes), VXLAN header (8 bytes)

	// Copy the rest of the frame
	dstHeaderStart := c.dstOffset()
	c.copyBytes(oldRecordLength - (c.srcOffset() - srcRecordStart))

	// XDR format requies 4-byte alignment, and the record headers
	// is the only variable-length record field
	headerLength := c.dstOffset() - dstHeaderStart
	if mod := headerLength % 4; mod != 0 {
		c.pad(4 - mod)
		headerLength += 4 - mod
	}

	// Update the record lenght field
	newRecordLength := uint32(c.dstOffset() - dstRecordStart)
	c.writeUint32At(newRecordLength, dstRecordStart-4)

	// Update the frameLength to reflect the absence of the stripped headers
	frameLength -= oldHeaderLength - uint32(headerLength)
	c.writeUint32At(frameLength, dstRecordStart+4)

	// Update the headerLength to reflect the absence of the stripped headers
	c.writeUint32At(uint32(headerLength), dstRecordStart+12)
}
