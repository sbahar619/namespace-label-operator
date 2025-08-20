#!/bin/bash

# Script to generate self-signed certificates for webhook
set -e

NAMESPACE=${1:-namespacelabel-system}
SERVICE_NAME="namespacelabel-webhook-service"
SECRET_NAME="webhook-server-cert"

echo "🔐 Generating webhook certificates for namespace: $NAMESPACE"

# Create temporary directory
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"

# Generate CA private key
openssl genrsa -out ca.key 2048

# Generate CA certificate
openssl req -new -x509 -key ca.key -out ca.crt -days 365 -subj "/CN=webhook-ca"

# Generate server private key
openssl genrsa -out server.key 2048

# Generate server certificate signing request
cat > server.conf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
CN = $SERVICE_NAME.$NAMESPACE.svc

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = $SERVICE_NAME
DNS.2 = $SERVICE_NAME.$NAMESPACE
DNS.3 = $SERVICE_NAME.$NAMESPACE.svc
DNS.4 = $SERVICE_NAME.$NAMESPACE.svc.cluster.local
EOF

# Generate server certificate signing request
openssl req -new -key server.key -out server.csr -config server.conf

# Generate server certificate
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -extensions v3_req -extfile server.conf

# Create namespace if it doesn't exist
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Delete existing secret if it exists
kubectl delete secret "$SECRET_NAME" -n "$NAMESPACE" --ignore-not-found=true

# Create secret with certificates
kubectl create secret tls "$SECRET_NAME" \
    --cert=server.crt \
    --key=server.key \
    -n "$NAMESPACE"

echo "🔍 Looking for ValidatingWebhookConfiguration..."

# Wait for webhook configuration to exist (in case deployment is still in progress)
for i in {1..30}; do
    WEBHOOK_CONFIG=$(kubectl get validatingwebhookconfigurations -o name 2>/dev/null | grep -E "(namespacelabel|validating)" | head -1)
    if [ -n "$WEBHOOK_CONFIG" ]; then
        break
    fi
    echo "   Waiting for ValidatingWebhookConfiguration... ($i/30)"
    sleep 2
done

# Update webhook configuration with CA bundle
CA_BUNDLE=$(base64 < ca.crt | tr -d '\n')

if [ -n "$WEBHOOK_CONFIG" ]; then
    echo "🔧 Updating $WEBHOOK_CONFIG with CA bundle..."
    kubectl patch "$WEBHOOK_CONFIG" \
        --type='json' \
        -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value': '$CA_BUNDLE'}]"
else
    echo "⚠️  No ValidatingWebhookConfiguration found after 60 seconds."
    echo "   The webhook may not be configured for validation."
fi

# Cleanup
cd - > /dev/null
rm -rf "$TEMP_DIR"

echo "✅ Webhook certificates created successfully!"
echo "📦 Secret '$SECRET_NAME' created in namespace '$NAMESPACE'"
if [ -n "$WEBHOOK_CONFIG" ]; then
    echo "🔗 ValidatingWebhookConfiguration updated with CA bundle"
fi
echo ""
echo "🚀 Webhook is ready for validation!" 