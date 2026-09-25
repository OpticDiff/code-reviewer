# Terraform Review Checklist

## Focus
- **Security**: Hardcoded secrets/credentials, overly broad IAM permissions, exposed security groups.
- **Reliability**: Unpinned provider versions.
- **State**: State drift risks, missing lifecycle blocks.

## DO NOT Flag
- Formatting (handled by `terraform fmt`).
- Naming conventions.
