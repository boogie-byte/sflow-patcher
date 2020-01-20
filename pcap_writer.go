package main

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type pcapWriter struct {
	handle *pcap.Handle
	srcMAC net.HardwareAddr
	dstMAC net.HardwareAddr
}

func newPcapWriter(ifName string, dstMAC net.HardwareAddr) (*pcapWriter, error) {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, err
	}

	handle, err := pcap.OpenLive(ifName, 1024, false, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	return &pcapWriter{
		handle: handle,
		srcMAC: iface.HardwareAddr,
		dstMAC: dstMAC,
	}, nil
}

func (w *pcapWriter) close() {
	w.handle.Close()
}

func (w *pcapWriter) write(srcAddr *net.UDPAddr, data []byte) error {
	dstAddr := routeMapLookup(srcAddr.IP)
	if dstAddr == nil {
		return fmt.Errorf("No collector configured for agent %s", srcAddr.IP.String())
	}

	buf := gopacket.NewSerializeBuffer()
	serializeOpts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	eth := &layers.Ethernet{
		SrcMAC:       w.srcMAC,
		DstMAC:       w.dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		SrcIP:    srcAddr.IP,
		DstIP:    dstAddr.IP,
		Protocol: layers.IPProtocolUDP,
		Version:  4,
		TTL:      32,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcAddr.Port),
		DstPort: layers.UDPPort(dstAddr.Port),
	}
	udp.SetNetworkLayerForChecksum(ip)

	payload := gopacket.Payload(data)

	if err := gopacket.SerializeLayers(buf, serializeOpts, eth, ip, udp, payload); err != nil {
		return err
	}

	return w.handle.WritePacketData(buf.Bytes())
}
