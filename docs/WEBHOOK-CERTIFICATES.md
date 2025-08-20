# Webhook Certificate Management

The NamespaceLabel operator includes admission webhooks that require TLS certificates for secure communication with the Kubernetes API server.

## Quick Start

For development and production, use the provided script to generate self-signed certificates:

```bash
# Generate and install webhook certificates
./hack/generate-webhook-certs.sh

# Deploy the operator
kubectl apply -f dist/install.yaml
```

The certificates are valid for 365 days.

## Certificate Management

### Generate Certificates

```bash
# Default namespace (namespacelabel-system)
./hack/generate-webhook-certs.sh

# Custom namespace
./hack/generate-webhook-certs.sh my-namespace
```

### What the script does:

1. **Generates self-signed CA and server certificates**
2. **Creates Kubernetes secret** `webhook-server-cert` with the TLS certificate
3. **Updates ValidatingWebhookConfiguration** with the CA bundle
4. **Configures proper DNS names** for the webhook service

### Certificate Renewal

Self-signed certificates expire after 365 days. To renew:

```bash
# Check expiration
kubectl get secret webhook-server-cert -n namespacelabel-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout | grep "Not After"

# Renew certificates (same command as initial generation)
./hack/generate-webhook-certs.sh
```

## Troubleshooting

Most webhook issues are resolved by regenerating certificates:
```bash
./hack/generate-webhook-certs.sh
```

Common checks:
```bash
# Check certificate exists
kubectl get secret webhook-server-cert -n namespacelabel-system

# Check controller is running  
kubectl get pods -n namespacelabel-system
```

## Security Considerations

- Certificates expire after 365 days - renew by re-running the script
- For production, consider implementing automated certificate rotation 