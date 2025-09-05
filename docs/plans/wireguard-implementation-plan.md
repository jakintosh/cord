# **Implementation Plan: internal/wireguard Package**

This document outlines the design, responsibilities, and implementation plan for the internal/wireguard package. This package is responsible for all interactions with the operating system's WireGuard implementation, including key generation, configuration management, and interface control.

## **1\. High-Level Application Architecture**

To ensure a clean separation of concerns and prevent state synchronization issues, the project will use a "daemon/controller" architecture.

* **cord-server (The Daemon):** This binary has one primary, long-running command: serve. This command is responsible for:  
  * Bringing the WireGuard network interface up (using the appropriate kernel or userspace backend).  
  * Serving the HTTP API.  
  * It is the **only process** that is allowed to modify the network's database or the live WireGuard interface after the initial setup.  
* **cord (The Controller/Client):** This binary has two roles:  
  * **Client:** For normal peers, commands like install, up, and sync manage the local machine's WireGuard interface and state.  
  * **Controller:** For administrators on the server machine, commands like cord server peer add act as simple HTTP clients. They send API requests to the running cord-server serve daemon to enact changes, ensuring that all modifications are handled by a single source of truth.

This model removes all direct database modification logic from the one-off admin commands and centralizes it in the serve daemon.

## **2\. Core Responsibilities of wireguard Package**

* **Key Management:** Generate, parse, and handle WireGuard public and private keys.  
* **State Representation:** Define Go structs that represent the complete state of a WireGuard interface, including the local device configuration and a list of all its peers.  
* **Configuration Generation:** "Compile" the internal Go structs into a native, wg-quick-compatible .conf file format for interoperability and debugging.  
* **Direct Interface Management:** Provide a high-level API to abstract the OS-level operations for creating, configuring, bringing up, and bringing down WireGuard interfaces directly using Go libraries, without shelling out to wg-quick.

## **3\. Data Persistence Model**

The wireguard package will manage interfaces directly in the kernel (on Linux) or in userspace and will also generate a native WireGuard configuration file (e.g., /etc/wireguard/prod-network.conf) as a persistent record of the intended state.

### **3.1. Rationale**

This hybrid approach provides the best of both worlds:

* **Ease of Use:** cord can manage the entire lifecycle of a WireGuard interface without requiring the user to install or manage external dependencies.  
* **Interoperability:** Generating the native .conf file makes the network state visible and debuggable with standard system tools (wg, wg-quick).  
* **Decoupling:** The network interface's lifecycle can still be managed by systemd at boot, even though cord manages it directly when running.

## **4\. Core Data Structures**

To achieve this, the package will export a primary struct to represent a WireGuard interface and its state.  
// Peer represents a single peer in a WireGuard configuration.  
type Peer struct {  
    PublicKey           wgtypes.Key  
    AllowedIPs          \[\]net.IPNet  
    Endpoint            \*net.UDPAddr  
    PersistentKeepalive time.Duration  
}

// Interface represents the complete configuration for a WireGuard network interface.  
// It holds all the state needed to apply to the kernel and generate a native .conf file.  
type Interface struct {  
    Name        string      // The interface name, e.g., "cord-prod"  
    PrivateKey  wgtypes.Key  
    Address     net.IPNet  
    ListenPort  int  
    Peers       \[\]Peer

    backend     Backend // Internal field for OS-specific implementation  
}

*(Note: Using native types like wgtypes.Key and net.IPNet internally will make working with the underlying libraries much cleaner.)*

## **5\. Primary API**

The package will expose a minimal, "all-in-one" interface for managing the lifecycle of a WireGuard interface.  
// NewInterface creates a new in-memory representation of a WireGuard interface.  
// It will select the appropriate OS backend automatically.  
func NewInterface(name string, privateKey wgtypes.Key, address net.IPNet, listenPort int) (\*Interface, error)

// AddPeer adds a peer to the interface's configuration.  
func (i \*Interface) AddPeer(peer Peer)

// RemovePeer removes a peer from the configuration by its public key.  
func (i \*Interface) RemovePeer(publicKey wgtypes.Key)

// Up creates the network device if it doesn't exist, configures it with the  
// current state of the Interface object, and brings it up.  
// It also writes the native .conf file to the specified path.  
func (i \*Interface) Up(configPath string) error

// Down brings the interface down and, optionally, deletes it.  
func (i \*Interface) Down(delete bool) error

// Sync applies only the changes to the peer list to a live interface  
// without tearing it down. This is more efficient for updates.  
func (i \*Interface) Sync() error

## **6\. Idempotency and Error Handling**

The Up function will handle existing interfaces gracefully using a "hot-reload" approach to minimize disruption.

1. **Fetch Current State:** Before applying any changes, the function will query the live interface to get its current configuration.  
2. **Compare States:** It will perform a deep comparison between the live configuration and the desired configuration stored in the Interface object.  
3. **No-Op on Match:** If the live and desired states are identical, the function will do nothing and return a success response.  
4. **Graceful Reconfiguration on Mismatch:** If the states differ, the function will atomically apply the new configuration. This process avoids tearing down the interface, preventing packet loss and connection resets.

## **7\. OS Abstraction**

To support Linux (kernel), macOS (userspace), and Windows (userspace), we will use a Backend interface with OS-specific implementations.  
// Backend defines the set of operations for a WireGuard implementation.  
type Backend interface {  
    Up(iface \*Interface, configPath string) error  
    Down(iface \*Interface, delete bool) error  
    Sync(iface \*Interface) error  
}

### **7.1. Implementation Strategy**

* **KernelBackend (Linux):**  
  * This will be the default implementation on Linux.  
  * It will use the vishvananda/netlink library to create and manage the network device.  
  * It will use the golang.zx2c4.com/wireguard/wgctrl library to configure the device's private key, listen port, and peers.  
  * This code will be guarded by a //go:build linux build tag.  
* **UserspaceBackend (macOS, Windows, and optional for Linux):**  
  * This will be the default for all non-Linux platforms.  
  * It will use the wireguard-go library to run the WireGuard protocol in userspace.  
  * It will require a helper library (like songgao/water or an internal equivalent) to create the virtual TUN network interface.  
  * The code for creating the TUN device will have its own OS-specific build tags (e.g., //go:build darwin for macOS, //go:build windows for Windows).

The NewInterface constructor will be responsible for detecting the OS and instantiating the correct Backend implementation. This completely hides the complexity from the rest of the cord application.

### **7.2. KernelBackend Internal Primitives**

The KernelBackend implementation will be built from a set of small, reusable, internal functions.

**Link Management (netlink):**

- `getLink(name string) (netlink.Link, error)`: Gets a link by name, return nil if doesn't exist.
- `ensureLink(name string) (netlink.Link, error)`: Idempotently ensures a WireGuard network interface with the given name exists.
- `setLinkUp(link netlink.Link) error` Ensures the interface is active ("up").
- `setLinkDown(link netlink.Link) error` Ensures the interface is inactive ("down").
- `removeLink(link netlink.Link) error` Deletes the interface.
- `syncAddress(link netlink.Link, addr net.IPNet) error` Ensures the interface has exactly the specified IP address, adding it if missing and removing any others.

**WireGuard Configuration (wgctrl):**
- `applyDeviceConfig(name string, key wgtypes.Key, port int) error`: Sets the device-specific WireGuard parameters (private key, listen port).
- `applyPeers(name string, peers []Peer) error`: Atomically replaces the entire peer list on the device.

### **7.3. KernelBackend Action Plan**

The public Backend methods will be implemented as compositions of the internal primitives defined above.

`Up(iface *Interface, configPath string)`

**Purpose:** Ensure the network interface exists, is fully configured, and is active.
- Call `ensureLink(iface.Name)` to get the link object.
- Call `syncAddress(link, iface.Address)`.
- Call `applyDeviceConfig(iface.Name, iface.PrivateKey, iface.ListenPort)`.
- Call `applyPeers(iface.Name, iface.Peers)`.
- Call `setLinkUp(link)`.
- Write the native config file to 1configPath1.

`Down(iface *Interface, delete bool)`

**Purpose:** Deactivate and optionally remove the network interface.
- Call `getLink(iface.Name)` to get the link object. If it doesn't exist, return success.
- Call `setLinkDown(link)`.
- If delete is true, call `removeLink(link)`.
- If delete is true, also remove the config file at `configPath`.

`Sync(iface *Interface)`

**Purpose:** Efficiently update only the peer list of an already running interface.
- Call `getLink(iface.Name)` to get the link object. If it doesn't exist or is not up, return an error.
- Call `applyPeers(iface.Name, iface.Peers)`.
