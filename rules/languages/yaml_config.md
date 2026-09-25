# YAML & K8s Config Review Checklist

## Focus
- **Reliability**: K8s manifests missing resource limits, missing liveness/readiness probes.
- **Security**: Latest image tags, missing security contexts, exposed NodePorts.
- **Helm**: Helm values without defaults.

## DO NOT Flag
- Indentation style.
- Comment formatting.
