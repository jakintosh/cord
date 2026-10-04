package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const dnsCleanupTimeout = 5 * time.Second

// DNSManager is the runtime's boundary to platform resolver configuration.
// The daemon supplies *dns.Manager, shared by the network-owned resolvers.
// Operations must honor cancellation and retain partially applied ownership
// until Remove succeeds. Runtime serializes calls through its owner loop.
type DNSManager interface {
	Apply(context.Context, dns.Config) error
	Remove(context.Context, dns.Config) error
	CleanupStale(context.Context, map[string]bool) error
}

// dnsResolver owns one network's resolver configuration across application,
// repair, and cleanup. Configuration is fixed at construction; the device
// supplies its live interface name. The network serializes lifecycle calls.
type dnsResolver struct {
	manager DNSManager
	device  *wireguard.Device
	config  dns.Config
	clock   func() time.Time

	// An attempted application may have changed the OS even when it failed.
	attempted *dns.Config
	lastErr   error

	applyStatus   resource.ActivityStatus
	cleanupStatus resource.ActivityStatus
}

// reconcile reapplies the desired configuration, removing a previous attempt
// first when the live interface name has changed.
func (d *dnsResolver) reconcile(
	ctx context.Context,
) (
	err error,
) {
	defer func() {
		d.lastErr = err
		d.applyStatus.Record(d.clock(), err)
	}()

	interfaceName, err := d.device.Name()
	if err != nil {
		return fmt.Errorf("DNS interface: %w", err)
	}

	if d.attempted != nil && d.attempted.Interface != interfaceName {
		if err := d.close(ctx); err != nil {
			return err
		}
	}

	// Apply may fail after changing the OS. Keep this attempt for cleanup.
	config := d.config
	config.Interface = interfaceName
	d.attempted = &config
	if err := d.manager.Apply(ctx, config); err != nil {
		return fmt.Errorf("apply DNS: %w", err)
	}
	return nil
}

// close retains failed cleanup for retry, including partially applied state.
func (d *dnsResolver) close(
	ctx context.Context,
) (
	err error,
) {
	if d.attempted == nil {
		return nil
	}
	defer func() {
		d.lastErr = err
		d.cleanupStatus.Record(d.clock(), err)
	}()

	if err := d.manager.Remove(ctx, *d.attempted); err != nil {
		return fmt.Errorf("remove DNS: %w", err)
	}
	d.attempted = nil
	return nil
}

// status reports saved outcomes without reading or changing OS configuration.
func (d *dnsResolver) status(
	target resource.State,
) resource.Snapshot {
	state := resource.Stopped
	switch {
	case d.lastErr != nil:
		state = resource.Failed
	case d.attempted != nil:
		state = resource.Running
	}

	status := resource.Snapshot{
		Kind:   "dns-resolver",
		Name:   d.config.Domain,
		State:  state,
		Target: target,
		Activities: map[string]resource.ActivityStatus{
			"apply": d.applyStatus,
			"close": d.cleanupStatus,
		},
	}
	if d.lastErr != nil {
		if errors.Is(d.lastErr, dns.ErrUnsupported) {
			status.Reason = "unsupported"
		}
		status.Error = d.lastErr.Error()
	}

	status.UpdateHealth()
	return status
}

// cleanupDNS runs only after a successful saved-state read. Failed startup
// cleanup is retried with a fresh active set on the next full maintenance pass.
func (r *Runtime) cleanupDNS(
	records []*service.Network,
) {
	if r.dnsCleanupDone {
		return
	}

	active := make(map[string]bool, len(records))
	for _, record := range records {
		if record.Enabled {
			active[record.Name+".internal"] = true
		}
	}

	err := r.dns.CleanupStale(r.ctx, active)
	r.dnsCleanupStatus.Record(r.clock(), err)
	if err != nil {
		r.log.Warn(
			"stale DNS cleanup failed",
			"err", err,
		)
		return
	}
	r.dnsCleanupDone = true
}
