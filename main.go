package main

import (
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var flagListenAddr string
var flagUpstreamAddr string
var flagOutIf string
var flagDstMAC string
var flagNumWorkers int
var flagMaxPayload int
var flagDebugEnabled bool

var rootCmd = &cobra.Command{
	Use: "sflow-patcher",
	Run: rootCmdHandler,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagUpstreamAddr, "upstream", "u", "", "upstream address:port")
	rootCmd.MarkPersistentFlagRequired("upstream")
	rootCmd.PersistentFlags().StringVarP(&flagOutIf, "out-if", "i", "", "outgoing interface")
	rootCmd.MarkPersistentFlagRequired("out-if")
	rootCmd.PersistentFlags().StringVarP(&flagDstMAC, "dst-mac", "m", "", "destination MAC address")
	rootCmd.MarkPersistentFlagRequired("dst-mac")

	rootCmd.PersistentFlags().StringVarP(&flagListenAddr, "bind", "b", "0.0.0.0:5000", "address and port to bind on")
	rootCmd.PersistentFlags().IntVarP(&flagNumWorkers, "workers", "w", 10, "number of workers")
	rootCmd.PersistentFlags().IntVarP(&flagMaxPayload, "buffer-size", "s", 1500, "input buffer size in bytes")
	rootCmd.PersistentFlags().BoolVarP(&flagDebugEnabled, "debug", "d", false, "enable debug logging")
}

func runWorker(conn *net.UDPConn, writer *pcapWriter, wg *sync.WaitGroup) {
	c := newCopier(flagMaxPayload)
	for {
		n, addr, err := conn.ReadFromUDP(c.src)
		c.reset(n)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				// see https://github.com/golang/go/issues/4373
				if opErr.Unwrap().Error() == "use of closed network connection" {
					break
				}
			}
			log.Error(err)
		}
		log.Debugf("Received %d bytes from %s", n, addr)

		data := c.process()
		if err := writer.write(addr, data); err != nil {
			log.Error(err)
		}
	}
	wg.Done()
}

func signalHandler(server net.PacketConn) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	log.Info(sig)
	server.Close()
}

func rootCmdHandler(_ *cobra.Command, _ []string) {
	if flagDebugEnabled {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging enabled")
	}

	// Create an outgoing interface handle for spoofing
	dstMAC, err := net.ParseMAC(flagDstMAC)
	if err != nil {
		log.Fatal(err)
	}
	dstAddr, err := net.ResolveUDPAddr("udp4", flagUpstreamAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Setting %s as outgoing interface", flagOutIf)
	writer, err := newPcapWriter(flagOutIf, dstMAC, dstAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer writer.close()

	// Set up UDP server
	srcAddr, err := net.ResolveUDPAddr("udp4", flagListenAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("Listening UDP on %s", srcAddr)
	conn, err := net.ListenUDP("udp4", srcAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Stop UDP server on SIGINT/SIGTERM
	go signalHandler(conn)

	log.Infof("Starting %d workers", flagNumWorkers)
	wg := &sync.WaitGroup{}
	wg.Add(flagNumWorkers)
	for i := 0; i < flagNumWorkers; i++ {
		go runWorker(conn, writer, wg)
	}

	wg.Wait()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
