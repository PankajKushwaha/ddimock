package main

import (
    "fmt"
)

func handleStatus(args []string) error {
    fmt.Println("Checking DDI service health...")

    services := []string{
        "IPAM",
        "IP-POOL",
        "DHCP",
        "DNS",
    }

    for _, service := range services {
        fmt.Printf("%-12s : UNKNOWN\n", service)
    }

    return nil
}

