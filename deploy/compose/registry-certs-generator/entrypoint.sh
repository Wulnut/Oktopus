#!/bin/sh
# Entrypoint for cert generator
# Generates certs if missing or if IPs have changed, then stays alive for healthcheck

set -e

CERT_DIR="${CERT_DIR:-/certs}"
FORCE_REGENERATE="${FORCE_REGENERATE:-false}"

# Function to detect current host IPs
detect_current_ips() {
  IPS=""
  for iface in $(ip -4 addr show 2>/dev/null | grep -E '^[0-9]+:' | awk '{print $2}' | cut -d: -f1 | grep -vE '^lo$|^docker|^veth|^br-|^virbr|^lxc' || true); do
    if_ips=$(ip -4 addr show "$iface" 2>/dev/null | grep -E 'inet ' | awk '{print $2}' | cut -d/ -f1 || true)
    if [ -n "$if_ips" ]; then
      for ip in $if_ips; do
        if echo "$ip" | grep -qvE '^127\.|^172\.(1[6-9]|2[0-9]|3[0-1])\.'; then
          IPS="$IPS $ip"
        fi
      done
    fi
  done
  
  if [ -z "$IPS" ]; then
    IPS=$(hostname -I 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i!~/^127\./ && $i!~/^172\.(1[6-9]|2[0-9]|3[0-1])\./) print $i}' | head -1 || true)
  fi
  
  IPS="127.0.0.1 $IPS"
  echo "$IPS" | tr ' ' '\n' | grep -v '^$' | sort -u | tr '\n' ' '
}

# Function to extract IPs from existing certificate
extract_cert_ips() {
  if [ ! -f "$CERT_DIR/server-cert.pem" ]; then
    echo ""
    return
  fi
  
  # Extract IP addresses from certificate SAN
  openssl x509 -in "$CERT_DIR/server-cert.pem" -noout -text 2>/dev/null | \
    grep -A1 "Subject Alternative Name" | \
    grep "IP Address" | \
    sed 's/.*IP Address://g' | \
    tr ',' '\n' | \
    sed 's/^[[:space:]]*//' | \
    grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | \
    sort -u | \
    tr '\n' ' ' || echo ""
}

needs_regeneration() {
  # Force regeneration if env var is set
  if [ "$FORCE_REGENERATE" = "true" ] || [ "$FORCE_REGENERATE" = "1" ]; then
    echo "Force regeneration requested"
    return 0
  fi
  
  # Check if certs don't exist
  if [ ! -f "$CERT_DIR/server-cert.pem" ] || [ ! -f "$CERT_DIR/server-key.pem" ] || [ ! -f "$CERT_DIR/ca.pem" ]; then
    echo "Certificates not found"
    return 0
  fi
  
  # Check if IPs have changed
  CURRENT_IPS=$(detect_current_ips)
  CERT_IPS=$(extract_cert_ips)
  
  # Normalize for comparison (sort and remove duplicates)
  CURRENT_NORMALIZED=$(echo "$CURRENT_IPS" | tr ' ' '\n' | sort -u | tr '\n' ' ')
  CERT_NORMALIZED=$(echo "$CERT_IPS" | tr ' ' '\n' | sort -u | tr '\n' ' ')
  
  if [ "$CURRENT_NORMALIZED" != "$CERT_NORMALIZED" ]; then
    echo "IP addresses changed. Current: [$CURRENT_NORMALIZED], Certificate: [$CERT_NORMALIZED]"
    return 0
  fi
  
  return 1
}

# Check if regeneration is needed
if needs_regeneration; then
  echo "Regenerating certificates..."
  # Remove old certs if they exist
  rm -f "$CERT_DIR"/*.pem "$CERT_DIR"/*.csr "$CERT_DIR"/*.cnf "$CERT_DIR"/*.srl 2>/dev/null || true
  /app/generate_certs.sh
  echo "Certificates generated successfully!"
else
  echo "Certificates are up to date. Skipping generation."
fi

# Stay alive for healthcheck
# Healthcheck will verify certs exist
echo "Cert generator ready. Waiting for healthcheck..."
while true; do
  sleep 60
done

