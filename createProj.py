#!/usr/bin/env python3
"""
create_structure.py
Creates the ddimock project directory structure with empty files.
All 'pool' identifiers renamed to 'ip-pool'.
"""

import os

ROOT = "ddimock"

# Directories to create
DIRS = [
    "proto/common",
    "proto/ip-pool",        # was proto/pool
    "proto/ipam",
    "proto/dns",
    "proto/dhcp",
    "internal/ip-pool",     # was internal/pool
    "internal/ipam",
    "internal/dns",
    "internal/dhcp",
    "cmd/ip-pool",          # was cmd/pool
    "cmd/ipam",
    "cmd/dns",
    "cmd/dhcp",
    "data",
]

# Files to create (relative to ROOT)
FILES = [
    "proto/common/types.proto",
    "proto/ip-pool/ip-pool.proto",     # was proto/pool/pool.proto
    "proto/ipam/ipam.proto",
    "proto/dns/dns.proto",
    "proto/dhcp/dhcp.proto",
    "internal/ip-pool/ip-pool.go",     # was internal/pool/pool.go
    "internal/ip-pool/server.go",      # was internal/pool/server.go
    "internal/ipam/server.go",
    "internal/dns/server.go",
    "internal/dhcp/server.go",
    "cmd/ip-pool/main.go",             # was cmd/pool/main.go
    "cmd/ipam/main.go",
    "cmd/dns/main.go",
    "cmd/dhcp/main.go",
    "data/ip-pool.txt",                # was data/pool.txt
    "docker-compose.yml",
    "Dockerfile",
    "go.mod",
    "README.md",
    ".gitignore",
    "LICENSE",
]


def main():
    os.makedirs(ROOT, exist_ok=True)
    print(f"Created directory: {ROOT}/")

    for d in DIRS:
        path = os.path.join(ROOT, d)
        os.makedirs(path, exist_ok=True)
        print(f"Created directory: {path}/")

    for f in FILES:
        path = os.path.join(ROOT, f)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        if not os.path.exists(path):
            with open(path, "w") as fp:
                pass
            print(f"Created file:      {path}")
        else:
            print(f"Exists, skipped:   {path}")

    print("\nDone. Project structure created under:", os.path.abspath(ROOT))


if __name__ == "__main__":
    main()
