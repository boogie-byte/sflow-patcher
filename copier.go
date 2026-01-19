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
	"encoding/binary"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

// copier holds source and destination byte slices and
// provides convenient methods for copying bytes from
// former to latter
type copier struct {
	// buf is a slice to which incoming packets are written
	src []byte
	// dst is a slice to which the data from src is being
	// copied to
	dst []byte
	// srcLen stores the original packet size
	srcLen int
	// srcOff stores current source read offset
	srcOff int
	// dstOff stores current destination write offset
	dstOff int
}

// returns new copier intance with source and destination
func newCopier(l int) *copier {
	return &copier{
		src: make([]byte, l),
		dst: make([]byte, l),
	}
}

// returns a slice containing processed data
func (c *copier) processedBytes() []byte {
	return c.dst[:c.dstOff]
}

// returns a slice with original packet payload
func (c *copier) sourceBytes() []byte {
	return c.src[:c.srcLen]
}

// returns current source offset
func (c *copier) srcOffset() int {
	return c.srcOff
}

// returns current destination offset
func (c *copier) dstOffset() int {
	return c.dstOff
}

// resets both source and destiantion offsets,
// sets source length to n
func (c *copier) reset(n int) {
	c.srcLen = n
	c.srcOff = 0
	c.dstOff = 0
}

// pads destination with n zeros
func (c *copier) pad(n int) {
	for i := 0; i < n; i++ {
		c.dst[c.dstOff+i] = 0
	}
	c.dstOff += n
}

// moves source offset n bytes
func (c *copier) skip(n int) {
	n += c.srcOff
	if n > len(c.src) {
		panic("source offset out of slice bounds")
	}
	if n < 0 {
		panic("source offset cannot be negative")
	}
	c.srcOff = n
}

// copies n bytes from source slice to destination
// slice and returns them
func (c *copier) copyBytes(n int) []byte {
	res := c.dst[c.dstOff : c.dstOff+n]
	copy(res, c.readBytes(n))
	c.dstOff += n
	return res
}

// copies n bytes from source slice to destination
// slice, starting with source offset srcOff and
// destination offset dstOff
func (c *copier) copyBytesAt(n, srcOff, dstOff int) {
	c.srcOff = srcOff + n
	c.dstOff = dstOff + n
	src := c.src[srcOff:c.srcOff]
	dst := c.dst[dstOff:c.dstOff]
	copy(dst, src)
}

// copies 4 bytes from source slice to destination
// slice and returns a big-endian uint32 decoded
// from them
func (c *copier) copyUint32() uint32 {
	return binary.BigEndian.Uint32(c.copyBytes(4))
}

// copies 4 data format bytes from source slice to
// destination slice and enterpriseID and dataType
// values decoded from them
func (c *copier) copyDataFormat() (uint32, uint32) {
	df := c.copyUint32()
	enterpriseID := df >> 12
	dataType := uint32(0xFFF) & uint32(df)
	return enterpriseID, dataType
}

// writes uint32 to destiantion slice at offset off
// without updating the destiantion offset
func (c *copier) writeUint32At(i uint32, off int) {
	// TODO check offset boundaries
	binary.BigEndian.PutUint32(c.dst[off:], i)
}

// returns n next bytes read from source slice
func (c *copier) readBytes(n int) []byte {
	res := c.src[c.srcOff : c.srcOff+n]
	c.srcOff += n
	return res
}

// reads 1 byte from source slice and returns its
// uint8 value
func (c *copier) readUint8() uint8 {
	return c.readBytes(1)[0]
}

// reads 2 bytes from source slice and returns
// their big-endian uint16 value
func (c *copier) readUint16() uint16 {
	return binary.BigEndian.Uint16(c.readBytes(2))
}

// reads 4 bytes from source slice and returns
// their big-endian uint32 value
func (c *copier) readUint32() uint32 {
	return binary.BigEndian.Uint32(c.readBytes(4))
}

// processes sflow datagram from the source slice
// into the destination slice
func (c *copier) process() (data []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("Failed to parse datagram: %s", r)
			log.Debugf("panic: %s\n%s", r, string(debug.Stack()))
			data = c.sourceBytes()
		} else {
			data = c.processedBytes()
		}
	}()
	processDatagram(c)
	return
}
