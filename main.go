package main

import (
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var listenAddr string
var numWorkers int
var maxPayload int
var debugEnabled bool
var rootCmd = &cobra.Command{
	Use:  "sflow-patcher <upstream address:port>",
	Run:  rootCmdHandler,
	Args: cobra.ExactArgs(1),
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&listenAddr, "bind", "b", "0.0.0.0:5000", "address and port to bind on")
	rootCmd.PersistentFlags().IntVarP(&numWorkers, "workers", "w", 10, "number of workers")
	rootCmd.PersistentFlags().IntVarP(&maxPayload, "buffer-size", "s", 1500, "input buffer size in bytes")
	rootCmd.PersistentFlags().BoolVarP(&debugEnabled, "debug", "d", false, "enable debug logging")
}

func handleParsingPanics(c *copier, upstream net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("Failed to parse datagram: %s", r)
			log.Debugf("panic: %s\n%s", r, string(debug.Stack()))
			upstream.Write(c.sourceBytes())
		} else {
			upstream.Write(c.processedBytes())
		}
	}()
	processDatagram(c)
}

func runWorker(conn net.PacketConn, upstream net.Conn, wg *sync.WaitGroup) {
	c := newCopier(maxPayload)
	for {
		if err := c.readPacket(conn); err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				// see https://github.com/golang/go/issues/4373
				if opErr.Unwrap().Error() == "use of closed network connection" {
					break
				}
			}
			log.Error(err)
		}
		handleParsingPanics(c, upstream)
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

func rootCmdHandler(_ *cobra.Command, args []string) {
	if debugEnabled {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging enabled")
	}

	// Create an upstream UDP connection handler
	log.Infof("Setting %s as destination", args[0])
	upstream, err := net.Dial("udp", args[0])
	if err != nil {
		log.Fatal(err)
	}
	defer upstream.Close()

	// Set up UDP server
	log.Infof("Listening UDP on %s", listenAddr)
	conn, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Stop UDP server on SIGINT/SIGTERM
	go signalHandler(conn)

	log.Infof("Starting %d workers", numWorkers)
	wg := &sync.WaitGroup{}
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go runWorker(conn, upstream, wg)
	}

	wg.Wait()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
