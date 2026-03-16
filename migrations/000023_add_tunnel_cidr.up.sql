-- Add tunnel_cidr to networks: separate VPN client address space from target network.
-- networks.address = target network routed through tunnel (goes into AllowedIPs)
-- networks.tunnel_cidr = VPN overlay subnet for client IPs (goes into Interface.Address)
-- If tunnel_cidr is NULL, address is used for both (legacy behavior).
ALTER TABLE networks ADD COLUMN tunnel_cidr CIDR;

-- Add comment for clarity.
COMMENT ON COLUMN networks.tunnel_cidr IS 'VPN overlay subnet for client tunnel IPs. If NULL, networks.address is used for both client IPs and AllowedIPs (legacy single-subnet mode).';
