
package main

import (
    "fmt"
)

func handleIP(args []string) error {
    if len(args) == 0 {
        return fmt.Errorf("usage: ddimock ip <list|show|allocate|release|reserve|quarantine>")
    }

    switch args[0] {
    case "list":
        fmt.Println("Listing IP addresses...")

    case "show":
        if len(args) < 2 {
            return fmt.Errorf("usage: ddimock ip show <IP>")
        }
        fmt.Println("Showing IP:", args[1])

    case "allocate":
        fmt.Println("Allocating IP address...")

    case "release":
        if len(args) < 2 {
            return fmt.Errorf("usage: ddimock ip release <IP>")
        }
        fmt.Println("Releasing IP:", args[1])

    case "reserve":
        fmt.Println("Reserving IP address...")

    case "quarantine":
        if len(args) < 2 {
            return fmt.Errorf("usage: ddimock ip quarantine <IP>")
        }
        fmt.Println("Quarantining IP:", args[1])

    default:
        return fmt.Errorf("unknown IP command: %s", args[0])
    }

    return nil
}

