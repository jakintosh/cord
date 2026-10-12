package dns

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"github.com/miekg/dns"
)

// White-box exception: a deliberately blocked library handler proves forced socket
// cleanup without adding a production hook or blocking the service/database.
func TestClose_ForcesCleanup(
	t *testing.T,
) {
	for _, protocol := range []string{"udp", "tcp"} {
		t.Run(protocol, func(t *testing.T) {
			packet, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { packet.Close() })
			listener, err := net.Listen("tcp", packet.LocalAddr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })

			entered := make(chan struct{})
			release := make(chan struct{})
			handler := dns.HandlerFunc(func(dns.ResponseWriter, *dns.Msg) {
				close(entered)
				<-release
			})
			started := make(chan struct{}, 2)
			notify := func() { started <- struct{}{} }
			srv := &Server{
				udp: &dns.Server{PacketConn: packet, Handler: handler, NotifyStartedFunc: notify},
				tcp: &dns.Server{Listener: listener, Handler: handler, NotifyStartedFunc: notify},
			}
			exited := make(chan error, 2)
			go func() { exited <- srv.udp.ActivateAndServe() }()
			go func() { exited <- srv.tcp.ActivateAndServe() }()
			for range 2 {
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("server did not start")
				}
			}
			t.Cleanup(func() {
				close(release)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = srv.Close(ctx)
				for range 2 {
					select {
					case err := <-exited:
						if err != nil {
							t.Error(err)
						}
					case <-ctx.Done():
						t.Error("library serving work did not finish after handler release")
						return
					}
				}
			})

			conn, err := net.Dial(protocol, listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.Close() })
			query := new(dns.Msg)
			query.SetQuestion("test.internal.", dns.TypeSOA)
			client := &dns.Conn{Conn: conn}
			if err := client.WriteMsg(query); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("handler did not receive query")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cancel()
			if err := srv.Close(ctx); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), protocol) {
				t.Fatalf("stop = %v, want %s cancellation", err, protocol)
			}

			if status := srv.Status(); status.State == resource.Running || status.Error == nil {
				t.Fatalf("status = %+v, want unavailable with shutdown error", status)
			}
			if err := srv.Close(context.Background()); err != nil {
				t.Fatalf("repeated stop: %v", err)
			}
			if protocol == "tcp" {
				conn.SetReadDeadline(time.Now().Add(time.Second))
				if _, err := client.ReadMsg(); err == nil {
					t.Fatal("active TCP connection survived Close")
				} else if e, ok := err.(net.Error); ok && e.Timeout() {
					t.Fatal("Close did not disconnect the TCP client")
				}
			}
			// The handler is still blocked, but both listening sockets are released.
			newTCP, err := net.Listen("tcp", listener.Addr().String())
			if err != nil {
				t.Fatalf("rebind TCP after stop: %v", err)
			}
			newTCP.Close()
			newUDP, err := net.ListenPacket("udp", packet.LocalAddr().String())
			if err != nil {
				t.Fatalf("rebind UDP after stop: %v", err)
			}
			newUDP.Close()
		})
	}
}
