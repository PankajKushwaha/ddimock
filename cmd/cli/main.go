package main

import (
    "fmt"
    "os"
)

func main() {
    if err := execute(os.Args[1:]); err != nil {
        fmt.Fprintln(os.Stderr, "Error:", err)
        os.Exit(1)
    }
}

func execute(args []string) error {

    if len(args) == 0 {
        printHelp()
        return nil
    }

    switch args[0] {
    case "network":
        return handleNetwork(args[1:])

    case "ip":
        return handleIP(args[1:])

    case "dns":
        return handleDNS(args[1:])

    case "dhcp":
        return handleDHCP(args[1:])

    case "host":
        return handleHost(args[1:])

    case "status":
        return handleStatus(args[1:])

    case "help", "--help", "-h":
        printHelp()
        return nil

    default:
        return fmt.Errorf("unknown command: %s", args[0])
    }
}

func printHelp() {
    fmt.Println(`
DDI Mock - Network Management CLI

Usage:
  ddimock <command> [subcommand] [flags]

Commands:
  network     Manage networks
  ip          Manage IP addresses
  dns         Manage DNS records
  dhcp        Manage DHCP scopes and leases
  host        Manage hosts
  status      Show service health

Options:
  help        Show help
`)
}

