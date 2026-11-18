
import sys
import json
import urllib.request
import urllib.error
import urllib.parse


# ============================================================
# SERVICE CONFIGURATION
# ============================================================

IP_POOL_URL = "http://localhost:5004"
IPAM_URL = "http://localhost:5001"
DHCP_URL = "http://localhost:5002"
DNS_URL = "http://localhost:5003"


# ============================================================
# COLORS
# ============================================================

GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
CYAN = "\033[96m"
RESET = "\033[0m"


# ============================================================
# HTTP HELPER
# ============================================================

def request(method, url, data=None):

    headers = {"Content-Type": "application/json"}
    body = None

    if data is not None:
        body = json.dumps(data).encode("utf-8")

    print("\n  " + CYAN + "[HTTP REQUEST]" + RESET)
    print("  Method : {}".format(method))
    print("  URL    : {}".format(url))

    if data is not None:
        print("  JSON   :")
        print(json.dumps(data, indent=4))

    req = urllib.request.Request(
        url,
        data=body,
        headers=headers,
        method=method
    )

    try:
        with urllib.request.urlopen(req, timeout=5) as response:

            content = response.read().decode("utf-8")
            result = json.loads(content) if content else {}

            print("\n  " + GREEN + "[HTTP RESPONSE]" + RESET)
            print("  Status : {}".format(response.status))
            print("  JSON   :")
            print(json.dumps(result, indent=4))

            return result

    except urllib.error.HTTPError as error:

        content = error.read().decode("utf-8")

        print("\n  " + RED + "[HTTP ERROR]" + RESET)
        print("  Status : {}".format(error.code))
        print("  Response:")
        print(content)

        raise

    except urllib.error.URLError as error:

        print("\n  " + RED + "[CONNECTION ERROR]" + RESET)
        print("  {}".format(error))

        raise


# ============================================================
# TEST HELPERS
# ============================================================

def print_step(message):
    print("\n" + YELLOW + message + RESET)


def success(message):
    print(GREEN + "[OK] " + message + RESET)


def failure(message):
    print(RED + "[FAILED] " + message + RESET)


def get_records(result, key):

    if isinstance(result, list):
        return result

    if isinstance(result, dict):
        records = result.get(key, [])
        if isinstance(records, list):
            return records

    return []


# ============================================================
# CHECK ALL MICROSERVICES
# ============================================================

def check_services():

    services = {
        "IP-POOL": IP_POOL_URL,
        "IPAM": IPAM_URL,
        "DHCP": DHCP_URL,
        "DNS": DNS_URL
    }

    print("\n===================================")
    print(" CHECKING MICROSERVICES")
    print("===================================")

    for name, url in services.items():

        try:
            result = request("GET", url + "/health")

            if result.get("status") != "UP":
                raise Exception("Service is not UP")

            success("{} is running".format(name))

        except Exception as error:
            failure("{} is not available: {}".format(name, error))
            print("\nStart all four microservices before running tests.")
            sys.exit(1)

    success("All four microservices are running.")


# ============================================================
# IPAM + DNS END-TO-END TEST
# ============================================================

def test_ipam_dns():

    hostname = "app-01.example.com"
    mac = "aa:bb:cc:00:00:50"
    network = "192.0.2.0/24"
    zone = "example.com"

    allocated_ip = None

    print("\n===================================")
    print(" IPAM + DNS END-TO-END TEST")
    print("===================================")

    try:

        # STEP 1: IPAM allocation

        print_step("1. Allocating IP through IPAM...")

        result = request(
            "POST",
            IPAM_URL + "/allocate",
            {
                "network": network,
                "hostname": hostname,
                "mac": mac
            }
        )

        allocated_ip = result.get("IP")

        if not allocated_ip:
            raise Exception("IPAM did not return an IP address")

        success("Allocated IP: {}".format(allocated_ip))


        # STEP 2: Create DNS zone

        print_step("2. Creating DNS zone...")

        request(
            "POST",
            DNS_URL + "/zones",
            {"zone": zone}
        )

        success("DNS zone created")


        # STEP 3: Create DNS record

        print_step("3. Creating DNS A record...")

        request(
            "POST",
            DNS_URL + "/records",
            {
                "name": hostname,
                "type": "A",
                "value": allocated_ip,
                "ttl": "300",
                "zone": zone
            }
        )

        success("DNS record created")


        # STEP 4: Verify IP allocation

        print_step("4. Verifying IP allocation...")

        result = request("GET", IPAM_URL + "/ips")
        records = get_records(result, "ips")

        ip_found = False

        for record in records:

            if (
                record.get("IP") == allocated_ip
                and record.get("Owner") == "ipam"
                and record.get("State") == "ALLOCATED"
            ):
                ip_found = True
                break

        if not ip_found:
            raise Exception("IP allocation verification failed")

        success("IP allocation verified")


        # STEP 5: Verify DNS record

        print_step("5. Verifying DNS record...")

        query = urllib.parse.urlencode({
            "name": hostname,
            "type": "A",
            "zone": zone
        })

        result = request(
            "GET",
            DNS_URL + "/records?" + query
        )

        if result.get("Value") != allocated_ip:
            raise Exception("DNS record verification failed")

        success(
            "DNS record verified: {} -> {}".format(
                hostname,
                allocated_ip
            )
        )


        # STEP 6: Release IP

        print_step("6. Releasing IP through IPAM...")

        request(
            "POST",
            IPAM_URL + "/release",
            {"ip": allocated_ip}
        )

        success("IP released")


        # STEP 7: Delete DNS record

        print_step("7. Deleting DNS record...")

        query = urllib.parse.urlencode({
            "name": hostname,
            "type": "A",
            "zone": zone
        })

        request(
            "DELETE",
            DNS_URL + "/records?" + query
        )

        success("DNS record deleted")

        print("\n" + GREEN + "IPAM + DNS TEST PASSED" + RESET)

    except Exception as error:

        failure("IPAM/DNS test failed: {}".format(error))

        if allocated_ip:
            print("Check whether IP {} is still allocated.".format(allocated_ip))

        raise


# ============================================================
# DHCP END-TO-END TEST
# ============================================================

def test_dhcp():

    hostname = "dhcp-client-01.example.com"
    mac = "aa:bb:cc:00:00:60"
    network = "192.0.2.0/24"

    lease_id = None
    allocated_ip = None

    print("\n===================================")
    print(" DHCP END-TO-END TEST")
    print("===================================")

    try:

        # STEP 1: Allocate DHCP lease

        print_step("1. Requesting IP through DHCP...")

        result = request(
            "POST",
            DHCP_URL + "/allocate",
            {
                "network": network,
                "hostname": hostname,
                "mac": mac,
                "duration_seconds": 120
            }
        )

        lease_id = result.get("id")
        allocated_ip = result.get("ip")

        if not lease_id or not allocated_ip:
            raise Exception("DHCP did not return lease ID and IP")

        success("DHCP allocated IP: {}".format(allocated_ip))
        success("Lease ID: {}".format(lease_id))


        # STEP 2: Verify DHCP lease

        print_step("2. Verifying DHCP lease...")

        result = request(
            "GET",
            DHCP_URL + "/leases"
        )

        leases = get_records(result, "leases")

        lease_found = False

        for lease in leases:

            if (
                lease.get("id") == lease_id
                and lease.get("ip") == allocated_ip
            ):
                lease_found = True
                break

        if not lease_found:
            raise Exception("DHCP lease not found")

        success("DHCP lease verified")


        # STEP 3: Verify IP Pool allocation

        print_step("3. Verifying IP Pool allocation...")

        result = request(
            "GET",
            IP_POOL_URL + "/ips"
        )

        records = get_records(result, "ips")

        ip_found = False

        for record in records:

            if (
                record.get("IP") == allocated_ip
                and record.get("Owner") == "dhcp"
                and record.get("State") == "ALLOCATED"
            ):
                ip_found = True
                break

        if not ip_found:
            raise Exception("DHCP IP allocation not found in IP Pool")

        success("IP Pool confirms DHCP allocation")


        # STEP 4: Renew DHCP lease

        print_step("4. Renewing DHCP lease...")

        result = request(
            "POST",
            DHCP_URL + "/renew",
            {
                "lease_id": lease_id,
                "duration_seconds": 300
            }
        )

        success("DHCP lease renewal request completed")


        # STEP 5: Verify renewed lease

        print_step("5. Verifying renewed DHCP lease...")

        result = request(
            "GET",
            DHCP_URL + "/leases"
        )

        leases = get_records(result, "leases")

        renewed = False

        for lease in leases:

            if (
                lease.get("id") == lease_id
                and lease.get("ip") == allocated_ip
            ):
                renewed = True
                print("  Renewed lease details:")
                print(json.dumps(lease, indent=4))
                break

        if not renewed:
            raise Exception("Renewed DHCP lease not found")

        success("DHCP lease renewal verified")


        # STEP 6: Release DHCP lease

        print_step("6. Releasing DHCP lease...")

        request(
            "POST",
            DHCP_URL + "/release",
            {
                "lease_id": lease_id
            }
        )

        success("DHCP lease released")


        # STEP 7: Verify lease removal

        print_step("7. Verifying DHCP lease removal...")

        result = request(
            "GET",
            DHCP_URL + "/leases"
        )

        leases = get_records(result, "leases")

        lease_still_exists = False

        for lease in leases:

            if lease.get("id") == lease_id:
                lease_still_exists = True
                break

        if lease_still_exists:
            raise Exception("Released lease still exists")

        success("DHCP lease removed")


        # STEP 8: Verify IP returned to pool

        print_step("8. Verifying IP returned to AVAILABLE state...")

        result = request(
            "GET",
            IP_POOL_URL + "/ips"
        )

        records = get_records(result, "ips")

        available = False

        for record in records:

            if (
                record.get("IP") == allocated_ip
                and record.get("State") == "AVAILABLE"
            ):
                available = True
                break

        if not available:
            raise Exception("Released IP did not return to AVAILABLE state")

        success(
            "IP {} returned to AVAILABLE state".format(allocated_ip)
        )

        print("\n" + GREEN + "DHCP TEST PASSED" + RESET)

    except Exception as error:

        failure("DHCP test failed: {}".format(error))

        if lease_id:
            print(
                "Check whether lease {} is still active.".format(lease_id)
            )

        raise


# ============================================================
# MAIN
# ============================================================

if __name__ == "__main__":

    print("\n===================================")
    print(" DDI MICROSERVICE INTEGRATION TEST")
    print("===================================")

    check_services()

    try:

        test_ipam_dns()

        test_dhcp()

        print("\n===================================")
        print(GREEN + " ALL TESTS COMPLETED SUCCESSFULLY" + RESET)
        print("===================================")

    except Exception:

        print("\n===================================")
        print(RED + " END-TO-END TEST FAILED" + RESET)
        print("===================================")

        sys.exit(1)

