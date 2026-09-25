# Protobuf Review Checklist

## Focus
- **Compatibility**: Wire compatibility (never reuse field numbers).
- **Safety**: Reserved fields, oneof safety.
- **Structure**: Enum zero value naming.
- **Lifecycle**: Deprecated field handling.
- **Contracts**: Breaking changes in API contracts.

## DO NOT Flag
- Formatting (handled by `buf format`).
- Naming style (handled by `buf lint`).
