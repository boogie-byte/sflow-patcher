package main

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type pcapWriter struct {
	handle  *pcap.Handle
	srcMAC  net.HardwareAddr
	dstMAC  net.HardwareAddr
	dstAddr *net.UDPAddr
}

func newPcapWriter(ifName string, dstMAC net.HardwareAddr, dstAddr *net.UDPAddr) (*pcapWriter, error) {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return nil, err
	}

	handle, err := pcap.OpenLive(ifName, 1024, false, pcap.BlockForever)
	if err != nil {
		return nil, err
	}

	return &pcapWriter{
		handle:  handle,
		srcMAC:  iface.HardwareAddr,
		dstMAC:  dstMAC,
		dstAddr: dstAddr,
	}, nil
}

func (w *pcapWriter) close() {
	w.handle.Close()
}

func (w *pcapWriter) write(srcAddr *net.UDPAddr, data []byte) error {
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
		DstIP:    w.dstAddr.IP,
		Protocol: layers.IPProtocolUDP,
		Version:  4,
		TTL:      32,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcAddr.Port),
		DstPort: layers.UDPPort(w.dstAddr.Port),
	}
	udp.SetNetworkLayerForChecksum(ip)

	payload := gopacket.Payload(data)

	if err := gopacket.SerializeLayers(buf, serializeOpts, eth, ip, udp, payload); err != nil {
		return err
	}

	return w.handle.WritePacketData(buf.Bytes())
}
