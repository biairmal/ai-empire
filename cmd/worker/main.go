// Command worker claims tasks from the control plane and runs them in isolated workspaces.
package main

import "log"

func main() {
	// Register/heartbeat/claim loop lands in M1.6.
	log.Println("worker: nothing to do yet")
}
