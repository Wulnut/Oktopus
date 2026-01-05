#!/bin/sh
# Automated certificate generation for Docker Registry
# Gets all non-docker interface IPs automatically
# Designed to run in container with host network access

set -e

CERT_DIR="${CERT_DIR:-/certs}"
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

echo "Generating certificates for Docker Registry..."
echo "Certificate directory: $CERT_DIR"

# Get all non-docker, non-loopback IPv4 addresses
# Excludes: docker*, lo, veth*, br-*, virbr*, lxc*
IPS=""
for iface in $(ip -4 addr show 2>/dev/null | grep -E '^[0-9]+:' | awk '{print $2}' | cut -d: -f1 | grep -vE '^lo$|^docker|^veth|^br-|^virbr|^lxc' || true); do
  if_ips=$(ip -4 addr show "$iface" 2>/dev/null | grep -E 'inet ' | awk '{print $2}' | cut -d/ -f1 || true)
  if [ -n "$if_ips" ]; then
    for ip in $if_ips; do
      # Exclude private Docker networks but keep other private IPs
      if echo "$ip" | grep -qvE '^127\.|^172\.(1[6-9]|2[0-9]|3[0-1])\.'; then
        IPS="$IPS $ip"
      fi
    done
  fi
done

# Fallback: if no IPs found, use hostname -I and filter
if [ -z "$IPS" ]; then
  IPS=$(hostname -I 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i!~/^127\./ && $i!~/^172\.(1[6-9]|2[0-9]|3[0-1])\./) print $i}' | head -1 || true)
fi

# Always include 127.0.0.1
IPS="127.0.0.1 $IPS"

# Remove duplicates and empty entries
IPS=$(echo "$IPS" | tr ' ' '\n' | grep -v '^$' | sort -u | tr '\n' ' ')

echo "Detected IP addresses: $IPS"

# Generate CA key (non-interactive with passphrase from env or default)
CA_PASSPHRASE="${CA_PASSPHRASE:-registry-ca-passphrase}"
echo "$CA_PASSPHRASE" | openssl genrsa -aes256 -passout stdin -out ca-key.pem 4096

# Generate CA certificate (non-interactive)
echo "$CA_PASSPHRASE" | openssl req -new -x509 -days 365 -key ca-key.pem -sha256 \
  -out ca.pem -passin stdin \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=Docker Registry CA"

# Generate server key
openssl genrsa -out server-key.pem 4096

# Generate server certificate signing request
openssl req -new -key server-key.pem -out server.csr \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=Docker Registry"

# Build subjectAltName extension value
SAN_IPS=""
for IP in $IPS; do
  if [ -n "$IP" ]; then
    if [ -z "$SAN_IPS" ]; then
      SAN_IPS="IP:$IP"
    else
      SAN_IPS="$SAN_IPS,IP:$IP"
    fi
  fi
done
SAN_IPS="$SAN_IPS,IP:127.0.0.1,IP:::1,DNS:localhost,DNS:registry"

# Create extfile in simple format (works with -extfile option)
cat > extfile.cnf <<EOF
subjectAltName = $SAN_IPS
extendedKeyUsage = serverAuth
EOF
cat extfile.cnf
# Generate server certificate
echo "$CA_PASSPHRASE" | openssl x509 -req -days 365 -sha256 \
  -in server.csr -CA ca.pem -CAkey ca-key.pem -CAcreateserial \
  -out server-cert.pem -extfile extfile.cnf -passin stdin

# Set proper permissions
chmod 600 ca-key.pem server-key.pem
chmod 644 ca.pem server-cert.pem

echo ""
echo "Certificates generated successfully!"
echo "CA certificate: $CERT_DIR/ca.pem"
echo "Server certificate: $CERT_DIR/server-cert.pem"
echo "Server key: $CERT_DIR/server-key.pem"

