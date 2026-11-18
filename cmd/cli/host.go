package main

import (
    "fmt"
)

func handleHost(args []string) error {
    if len(args) == 0 || args[0] != "show" {
        return fmt.Errorf("usage: ddimock host show <hostname>")
    }

    if len(args) < 2 {
        return fmt.Errorf("hostname is required")
    }

    fmt.Println("Host details:", args[1])

    return nil
}

