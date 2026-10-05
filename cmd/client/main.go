package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	pb "vswitch/tunnel"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:50051", "vswitch gRPC address")
	ca := flag.String("ca", "", "CA cert file (empty=insecure)")
	service := flag.String("service", "android-client", "service name")
	tunName := flag.String("tun", "vswitch0", "TUN interface name")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	log.Printf("[client] addr=%s service=%s tun=%s tls=%v", *addr, *service, *tunName, *ca != "")
	for {
		if err := run(ctx, *addr, *ca, *service, *tunName); err != nil {
			log.Printf("[client] error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
			log.Printf("[client] reconnecting...")
		}
	}
}

func run(ctx context.Context, addr, caFile, service, tunName string) error {
	var opts []grpc.DialOption
	if caFile == "" {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return fmt.Errorf("read CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid CA cert")
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: pool})))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	client := pb.NewTunnelServiceClient(conn)
	md := metadata.Pairs("x-service-name", service)
	stream, err := client.Stream(metadata.NewOutgoingContext(ctx, md))
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	log.Printf("[client] connected")
	tun, err := openTUN(tunName)
	if err != nil {
		return fmt.Errorf("tun: %w", err)
	}
	defer tun.Close()
	myMAC, myIP, gwIP, gwMAC, err := bootstrap(stream)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	log.Printf("[client] MAC=%s IP=%s GW=%s GW-MAC=%s", fmtMAC(myMAC), fmtIP(myIP), fmtIP(gwIP), fmtMAC(gwMAC))
	configureTUN(tunName, myIP)
	errc := make(chan error, 2)
	go func() { // gRPC -> TUN
		for {
			frame, err := stream.Recv()
			if err != nil {
				errc <- fmt.Errorf("recv: %w", err)
				return
			}
			b := frame.GetPayload()
			if len(b) < 14 {
				continue
			}
			if !isBcast(b[:6]) && !eq6(b[:6], myMAC[:]) {
				continue
			}
			if binary.BigEndian.Uint16(b[12:14]) == 0x0806 && len(b) >= 42 {
				if binary.BigEndian.Uint16(b[20:22]) == 1 && eq4(b[38:42], myIP[:]) {
					_ = stream.Send(&pb.Frame{Payload: arpReply(myMAC, myIP, b[6:12], b[28:32])})
				}
				continue
			}
			if binary.BigEndian.Uint16(b[12:14]) != 0x0800 {
				continue
			}
			if _, err := tun.Write(b[14:]); err != nil {
				errc <- fmt.Errorf("tun write: %w", err)
				return
			}
		}
	}()
	go func() { // TUN -> gRPC
		buf := make([]byte, 2048)
		for {
			n, err := tun.Read(buf)
			if err != nil {
				errc <- fmt.Errorf("tun read: %w", err)
				return
			}
			if n < 20 {
				continue
			}
			f := make([]byte, 14+n)
			copy(f[:6], gwMAC[:])
			copy(f[6:12], myMAC[:])
			binary.BigEndian.PutUint16(f[12:14], 0x0800)
			copy(f[14:], buf[:n])
			if err := stream.Send(&pb.Frame{Payload: f}); err != nil {
				errc <- fmt.Errorf("send: %w", err)
				return
			}
		}
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func bootstrap(stream grpc.BidiStreamingClient[pb.Frame, pb.Frame]) ([6]byte, [4]byte, [4]byte, [6]byte, error) {
	var myMAC [6]byte
	myMAC[0] = 0x02
	for i := 1; i < 6; i++ {
		myMAC[i] = byte(time.Now().UnixNano() >> (uint(i) * 8))
	}
	var myIP, gwIP [4]byte
	var gwMAC [6]byte
	disc := dhcpDiscover(myMAC)
	if err := stream.Send(&pb.Frame{Payload: disc}); err != nil {
		return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("dhcp send: %w", err)
	}
	dl := time.After(10 * time.Second)
	for {
		select {
		case <-dl:
			return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("dhcp timeout")
		default:
		}
		f, err := stream.Recv()
		if err != nil {
			return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("dhcp recv: %w", err)
		}
		b := f.GetPayload()
		if len(b) < 70 || binary.BigEndian.Uint16(b[12:14]) != 0x0800 || b[23] != 17 {
			continue
		}
		d := 42
		if len(b) < d+24 {
			continue
		}
		myIP = [4]byte{b[d+16], b[d+17], b[d+18], b[d+19]}
		gwIP = [4]byte{b[d+20], b[d+21], b[d+22], b[d+23]}
		break
	}
	arp := arpRequest(myMAC, myIP, gwIP)
	if err := stream.Send(&pb.Frame{Payload: arp}); err != nil {
		return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("arp send: %w", err)
	}
	dl = time.After(10 * time.Second)
	for {
		select {
		case <-dl:
			return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("arp timeout")
		default:
		}
		f, err := stream.Recv()
		if err != nil {
			return myMAC, myIP, gwIP, gwMAC, fmt.Errorf("arp recv: %w", err)
		}
		b := f.GetPayload()
		if len(b) < 42 || binary.BigEndian.Uint16(b[12:14]) != 0x0806 || binary.BigEndian.Uint16(b[20:22]) != 2 {
			continue
		}
		copy(gwMAC[:], b[22:28])
		break
	}
	return myMAC, myIP, gwIP, gwMAC, nil
}

func openTUN(name string) (*os.File, error) {
	fd, err := syscall.Open("/dev/net/tun", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var ifr [40]byte
	copy(ifr[:16], name)
	*(*uint16)(unsafe.Pointer(&ifr[16])) = syscall.IFF_TUN | syscall.IFF_NO_PI
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x400454ca, uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		syscall.Close(fd)
		return nil, errno
	}
	return os.NewFile(uintptr(fd), "/dev/net/tun"), nil
}

func configureTUN(name string, ip [4]byte) {
	addr := fmt.Sprintf("%d.%d.%d.%d/24", ip[0], ip[1], ip[2], ip[3])
	for _, args := range [][]string{
		{"ip", "addr", "replace", addr, "dev", name},
		{"ip", "link", "set", name, "up"},
	} {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			log.Printf("[client] %v: %v %s", args, err, out)
		}
	}
}

func dhcpDiscover(mac [6]byte) []byte {
	dl := 240
	b := make([]byte, 14+20+8+dl)
	copy(b[:6], []byte{255, 255, 255, 255, 255, 255})
	copy(b[6:12], mac[:])
	binary.BigEndian.PutUint16(b[12:14], 0x0800)
	b[14] = 0x45
	binary.BigEndian.PutUint16(b[16:18], uint16(20+8+dl))
	b[22] = 64
	b[23] = 17
	copy(b[30:34], []byte{255, 255, 255, 255})
	binary.BigEndian.PutUint16(b[24:26], ipck(b[14:34]))
	binary.BigEndian.PutUint16(b[34:36], 68)
	binary.BigEndian.PutUint16(b[36:38], 67)
	binary.BigEndian.PutUint16(b[38:40], uint16(8+dl))
	d := 42
	b[d] = 1
	b[d+1] = 1
	b[d+2] = 6
	copy(b[d+28:d+34], mac[:])
	copy(b[d+236:d+240], []byte{0x63, 0x82, 0x53, 0x63})
	b[d+240], b[d+241], b[d+242] = 53, 1, 1
	b[d+243] = 255
	return b
}

func arpRequest(src [6]byte, sip, dip [4]byte) []byte {
	b := make([]byte, 42)
	copy(b[:6], []byte{255, 255, 255, 255, 255, 255})
	copy(b[6:12], src[:])
	binary.BigEndian.PutUint16(b[12:14], 0x0806)
	binary.BigEndian.PutUint16(b[14:16], 1)
	binary.BigEndian.PutUint16(b[16:18], 0x0800)
	b[18], b[19] = 6, 4
	binary.BigEndian.PutUint16(b[20:22], 1)
	copy(b[22:28], src[:])
	copy(b[28:32], sip[:])
	copy(b[38:42], dip[:])
	return b
}

func arpReply(src [6]byte, sip [4]byte, dm, dip []byte) []byte {
	b := make([]byte, 42)
	copy(b[:6], dm)
	copy(b[6:12], src[:])
	binary.BigEndian.PutUint16(b[12:14], 0x0806)
	binary.BigEndian.PutUint16(b[14:16], 1)
	binary.BigEndian.PutUint16(b[16:18], 0x0800)
	b[18], b[19] = 6, 4
	binary.BigEndian.PutUint16(b[20:22], 2)
	copy(b[22:28], src[:])
	copy(b[28:32], sip[:])
	copy(b[32:38], dm)
	copy(b[38:42], dip)
	return b
}

func ipck(b []byte) uint16 {
	var s uint32
	for i := 0; i+1 < len(b); i += 2 {
		s += uint32(binary.BigEndian.Uint16(b[i:]))
	}
	for s>>16 != 0 {
		s = (s & 0xffff) + (s >> 16)
	}
	return ^uint16(s)
}

func isBcast(m []byte) bool { return len(m) >= 6 && m[0] == 255 && m[1] == 255 && m[2] == 255 && m[3] == 255 && m[4] == 255 && m[5] == 255 }
func eq6(a, b []byte) bool   { return len(a) >= 6 && len(b) >= 6 && a[0] == b[0] && a[1] == b[1] && a[2] == b[2] && a[3] == b[3] && a[4] == b[4] && a[5] == b[5] }
func eq4(a, b []byte) bool   { return len(a) >= 4 && len(b) >= 4 && a[0] == b[0] && a[1] == b[1] && a[2] == b[2] && a[3] == b[3] }
func fmtMAC(m [6]byte) string {
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", m[0], m[1], m[2], m[3], m[4], m[5])
}
func fmtIP(ip [4]byte) string { return fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]) }