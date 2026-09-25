# phi-free-event-broker

## Focus
AutoMQ and Kafka streaming topics must carry opaque identifiers, trigger event
codes, and classification metadata only — never raw patient names, clinical narrative
text, or unredacted EHR/HL7 payloads. The broker must remain strictly outside the
HIPAA compliance and PHI retention blast radius.
Downstream consumers must use the Claim-Check pattern: receive opaque identifiers
(patient_id, encounter_id, document_uuid) and fetch clinical content via an
authenticated, audited ConnectRPC call or short-lived pre-signed URL.
Beware of HL7 ACK/NAK segment echoes (e.g., ERR/MSA echoing the PID segment)
which leak PHI back onto event topics during parsing failures.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/phi-free-event-broker
