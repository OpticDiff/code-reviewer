# Safe JSON Handling

## Focus
- `json.Unmarshal` on LLM output MUST be wrapped in a repair-and-validate pipeline. Raw unmarshal on LLM responses is fragile.
- `json.Decoder.DisallowUnknownFields()` SHOULD be used when decoding into strict structs (webhook payloads, config files).
- JSON error messages MUST NOT leak raw request bodies into logs or error responses.
- Large JSON responses SHOULD use `json.Decoder` streaming instead of `json.Unmarshal` on the full body.

## DO NOT Flag
- `json.Marshal` (encoding direction is safe).
- Small, trusted internal structs.
- Test data unmarshaling.
