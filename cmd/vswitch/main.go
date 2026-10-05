package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
	"vswitch/pkg/api"
	"vswitch/pkg/config"
	"vswitch/pkg/core"
	"vswitch/pkg/dhcp"
	"vswitch/pkg/emulation"
	"vswitch/pkg/metrics"
	"vswitch/pkg/netstack"
	"vswitch/pkg/transport"
)

func parseMAC(s string) core.MACAddr {
	var m core.MACAddr
	parts := strings.Split(s, ":")
	if len(parts) == 6 {
		for i, p := range parts {
			b, _ := fmt.Sscanf(p, "%02x", &m[i])
			_ = b
		}
	}
	return m
}
func parseIP(s string) [4]byte {
	var x [4]byte
	_, _ = fmt.Sscanf(s, "%d.%d.%d.%d", &x[0], &x[1], &x[2], &x[3])
	return x
}
func setUDSBuffers(c net.Conn) {
	if u, ok := c.(*net.UnixConn); ok {
		raw, e := u.SyscallConn()
		if e == nil {
			_ = raw.Control(func(fd uintptr) { _ = fd })
		}
	}
}
func main() {
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, e := config.LoadConfig(cfgPath)
	if e != nil {
		log.Fatal(e)
	}
	m := &metrics.Metrics{}
	store := dhcp.NewStore(cfg.Storage.LeasesFile)
	if e = store.Load(); e != nil {
		log.Printf("lease load: %v", e)
	}
	gwip := parseIP(cfg.Network.GatewayIP)
	gwmac := parseMAC(cfg.Network.GatewayMAC)
	arp := &dhcp.ARPHandler{GatewayIP: gwip, GatewayMAC: [6]byte(gwmac)}
	d := &dhcp.Server{GatewayIP: gwip, GatewayMAC: [6]byte(gwmac), Store: store}
	sw := core.New(m, arp, d)
	bridge := netstack.New(sw, gwip, gwmac)
	sw.Netstack = bridge
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	bridge.Start(ctx)
	uds := &transport.UDSFile{Path: cfg.Transports.UDSFile, Switch: sw}
	if e = uds.Start(ctx); e != nil {
		log.Printf("UDS file: %v", e)
	}
	abs := &transport.UDSAbstract{Name: cfg.Transports.UDSAbstract, Switch: sw}
	if e = abs.Start(ctx); e != nil {
		log.Printf("UDS abstract: %v", e)
	}
	udp := &transport.UDP{Addr: cfg.Transports.UDP, Switch: sw}
	if e = udp.Start(ctx); e != nil {
		log.Printf("UDP: %v", e)
	}
	grpcSrv := &transport.GRPC{Addr: cfg.Transports.GRPC, TLSCert: cfg.Transports.TLSCert, TLSKey: cfg.Transports.TLSKey, Switch: sw}
	if e = grpcSrv.Start(ctx); e != nil {
		log.Printf("gRPC: %v", e)
	}
	for _, e := range cfg.Emulation.Profiles {
		var prof emulation.Profile
		if e.Profile != nil {
			prof = *e.Profile
		}
		if e.Preset != "" && e.Profile == nil {
			prof = emulation.Preset(e.Preset)
		}
		if err := sw.ApplyEmulation(e.Key, prof); err != nil {
			log.Printf("emulation config (%s): %v", e.Key, err)
		}
	}
	var webSrv *http.Server
	if cfg.WebUI.Enabled {
		h := &api.Handlers{Metrics: m, Switch: sw, LeaseStore: store, ConfigPath: cfgPath, Cfg: cfg}
		webSrv = &http.Server{Addr: cfg.WebUI.ListenAddr, Handler: (&api.Router{H: h}).Handler(), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			log.Printf("WebUI/API: %s", cfg.WebUI.ListenAddr)
			if e := webSrv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
				log.Printf("web: %v", e)
			}
		}()
	}
	log.Printf("[L2 SWITCH MULTI-TRANSPORT ACTIVE]")
	log.Printf("File UDS: %s", cfg.Transports.UDSFile)
	log.Printf("Abstract UDS: %s", cfg.Transports.UDSAbstract)
	log.Printf("UDP: %s", cfg.Transports.UDP)
	log.Printf("gRPC: %s (TLS=%v)", cfg.Transports.GRPC, cfg.Transports.TLSCert != "")
	<-ctx.Done()
	shctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if webSrv != nil {
		_ = webSrv.Shutdown(shctx)
	}
	uds.Close()
	abs.Close()
	udp.Close()
	grpcSrv.Close()
	sw.Emulation.StopAll()
	_ = store.Save()
	_ = os.Remove(cfg.Transports.UDSFile)
	log.Printf("shutdown complete; goroutines=%d", runtime.NumGoroutine())
}

var _ = hex.EncodeToString
var _ = setUDSBuffers
