package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/go-yaml/yaml"
)

type routeMap struct {
	d *net.UDPAddr            // deafult route
	r map[uint32]*net.UDPAddr // routes map
}

func newRouteMap() *routeMap {
	return &routeMap{
		r: make(map[uint32]*net.UDPAddr),
	}
}

func (m *routeMap) Set(agentIP net.IP, collAddr *net.UDPAddr) {
	ipUint := binary.BigEndian.Uint32(agentIP)
	m.r[ipUint] = collAddr
}

func (m *routeMap) Get(agentIP net.IP) *net.UDPAddr {
	ipUint := binary.BigEndian.Uint32(agentIP)
	if val, ok := m.r[ipUint]; ok {
		return val
	}
	return m.d
}

var routeMapPointer unsafe.Pointer

func routeMapLookup(agentIP net.IP) *net.UDPAddr {
	return (*routeMap)(atomic.LoadPointer(&routeMapPointer)).Get(agentIP)
}

func routeMapReload() error {
	data, err := ioutil.ReadFile(flagRouteMapPath)
	if err != nil {
		return err
	}

	fileMap := make(map[string]string)
	if err := yaml.Unmarshal(data, fileMap); err != nil {
		return err
	}

	l := newRouteMap()
	for k, v := range fileMap {
		if strings.ToLower(k) == "default" {
			collAddr, err := net.ResolveUDPAddr("udp4", v)
			if err != nil {
				return err
			}
			l.d = collAddr
			continue
		}

		agentIP := net.ParseIP(k)
		if agentIP == nil {
			return fmt.Errorf("Cannot parse agent address %s", k)
		}
		agentIP = agentIP.To4()
		if agentIP == nil {
			return fmt.Errorf("Only IPv4 agent addresses are supported")
		}

		collAddr, err := net.ResolveUDPAddr("udp4", v)
		if err != nil {
			return err
		}

		l.Set(agentIP, collAddr)
	}

	atomic.StorePointer(&routeMapPointer, unsafe.Pointer(l))

	return nil
}
