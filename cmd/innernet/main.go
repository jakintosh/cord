package main

import (
	"fmt"
	"os"
)

func main() {

	// what does (client) program do
	//
	// * start and stop wireguard interfaces
	// * talks to server to generate wireguard configs
	// * "gossip" peer endpoints with server

	args := os.Args[1:]

	if len(args) < 1 {
		fmt.Printf("nothing to do\n")
		return
	}

	// switch args[0] {

	// case "init":
	// 	createInterface("wg0")

	// case "uninstall":
	// 	deleteInterface("wg0")

	// case "list":
	// 	listInterfaces()

	// case "peer":
	// 	args := os.Args[1:]
	// 	if len(args) < 1 {
	// 		fmt.Printf("peer help\n")
	// 	} else {
	// 		switch args[1] {
	// 		case "add":
	// 			fmt.Printf("new peer\n")
	// 		case "rename":
	// 			fmt.Printf("rename peer\n")
	// 		case "disable":
	// 			fmt.Printf("disable peer\n")
	// 		case "enable":
	// 			fmt.Printf("enable peer\n")
	// 		default:
	// 			fmt.Printf("unhandled peer command %s\n", args[1])
	// 		}
	// 	}

	// default:
	// 	fmt.Printf("unhandled command %s\n", args[0])
	// }
}
